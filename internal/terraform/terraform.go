package terraform

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Runner wraps terraform CLI interactions for a given working directory.
type Runner struct {
	WorkDir string
	Binary  string // "terraform" or "tofu"
}

// NewRunner creates a Runner, auto-detecting tofu vs terraform.
func NewRunner(workDir string) *Runner {
	binary := "terraform"
	if _, err := exec.LookPath("tofu"); err == nil {
		// prefer tofu if available, but check if there's a .terraform dir
		// indicating terraform was used
		if _, err := os.Stat(filepath.Join(workDir, ".terraform")); err != nil {
			binary = "tofu"
		}
	}
	return &Runner{WorkDir: workDir, Binary: binary}
}

// Resource represents a resource in terraform state.
type Resource struct {
	Type     string
	Name     string
	Module   string
	Address  string
	Provider string
}

// TfFile represents a terraform file in the working directory.
type TfFile struct {
	Name    string // basename (e.g. "main.tf")
	Path    string // full absolute path
	RelPath string // relative to workdir (e.g. "modules/druid/main.tf")
	Dir     string // relative directory (e.g. "modules/druid", "" for root)
	IsVars  bool
	Size    int64
	Content string
	Depth   int // nesting depth (0 = root)
}

// Output represents a terraform output value.
type Output struct {
	Name      string
	Value     any
	Type      string
	Sensitive bool
}

// WorkspaceInfo holds workspace details.
type WorkspaceInfo struct {
	Current    string
	Workspaces []string
}

// run executes a terraform command and returns combined output.
func (r *Runner) run(args ...string) (string, error) {
	cmd := exec.Command(r.Binary, args...)
	cmd.Dir = r.WorkDir
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1", "TF_CLI_ARGS=-no-color")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runStream executes a terraform command and streams output line by line.
func (r *Runner) runStream(onLine func(string), args ...string) error {
	cmd := exec.Command(r.Binary, args...)
	cmd.Dir = r.WorkDir
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1", "TF_CLI_ARGS=-no-color")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		onLine(scanner.Text())
	}

	return cmd.Wait()
}

// Init runs terraform init.
func (r *Runner) Init() (string, error) {
	return r.run("init")
}

// Plan runs terraform plan with optional var-file.
func (r *Runner) Plan(varFile string) (string, error) {
	args := []string{"plan"}
	if varFile != "" {
		args = append(args, "-var-file="+varFile)
	}
	return r.run(args...)
}

// PlanStream runs terraform plan and streams output.
func (r *Runner) PlanStream(varFile string, onLine func(string)) error {
	args := []string{"plan"}
	if varFile != "" {
		args = append(args, "-var-file="+varFile)
	}
	return r.runStream(onLine, args...)
}

// PlanSaveStream runs terraform plan with -out to save a plan file, streaming output.
// If destroy is true, adds -destroy flag for a destroy plan.
func (r *Runner) PlanSaveStream(varFile, outFile string, destroy bool, onLine func(string)) error {
	args := []string{"plan", "-out=" + outFile}
	if destroy {
		args = append(args, "-destroy")
	}
	if varFile != "" {
		args = append(args, "-var-file="+varFile)
	}
	return r.runStream(onLine, args...)
}

// PlanTargetSaveStream runs terraform plan with -target flags and -out to save
// a plan file, streaming output. Used for targeted plan → review → apply flows.
func (r *Runner) PlanTargetSaveStream(varFile, outFile string, targets []string, onLine func(string)) error {
	args := []string{"plan", "-out=" + outFile}
	for _, t := range targets {
		args = append(args, "-target="+t)
	}
	if varFile != "" {
		args = append(args, "-var-file="+varFile)
	}
	return r.runStream(onLine, args...)
}

// ApplyPlanStream runs terraform apply on a saved plan file, streaming output.
// No -auto-approve is needed because applying a saved plan is non-interactive.
func (r *Runner) ApplyPlanStream(planFile string, onLine func(string)) error {
	return r.runStream(onLine, "apply", planFile)
}

// Validate runs terraform validate.
func (r *Runner) Validate() (string, error) {
	return r.run("validate")
}

// Fmt runs terraform fmt check.
func (r *Runner) Fmt() (string, error) {
	return r.run("fmt", "-check", "-diff")
}

// FmtFix runs terraform fmt to fix files.
func (r *Runner) FmtFix() (string, error) {
	return r.run("fmt")
}

// StateList returns all resources in state.
func (r *Runner) StateList() ([]Resource, error) {
	out, err := r.run("state", "list")
	if err != nil {
		// No state yet is not an error for our UI
		if strings.Contains(out, "No state file") || strings.Contains(out, "does not exist") {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", out, err)
	}

	var resources []Resource
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		res := parseResourceAddress(line)
		resources = append(resources, res)
	}
	return resources, nil
}

// StateShow returns details of a specific resource.
func (r *Runner) StateShow(address string) (string, error) {
	return r.run("state", "show", address)
}

// StateTaint marks a resource for recreation.
func (r *Runner) StateTaint(address string) (string, error) {
	return r.run("taint", address)
}

// StateUntaint unmarks a resource.
func (r *Runner) StateUntaint(address string) (string, error) {
	return r.run("untaint", address)
}

// StateRm removes a resource from state.
func (r *Runner) StateRm(address string) (string, error) {
	return r.run("state", "rm", address)
}

// Workspaces returns workspace info.
func (r *Runner) Workspaces() (*WorkspaceInfo, error) {
	out, err := r.run("workspace", "list")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", out, err)
	}

	info := &WorkspaceInfo{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "* ") {
			name := strings.TrimPrefix(line, "* ")
			info.Current = name
			info.Workspaces = append(info.Workspaces, name)
		} else {
			info.Workspaces = append(info.Workspaces, line)
		}
	}
	return info, nil
}

// WorkspaceSelect switches to a workspace.
func (r *Runner) WorkspaceSelect(name string) (string, error) {
	return r.run("workspace", "select", name)
}

// WorkspaceNew creates a new workspace.
func (r *Runner) WorkspaceNew(name string) (string, error) {
	return r.run("workspace", "new", name)
}

// WorkspaceDelete deletes a workspace.
func (r *Runner) WorkspaceDelete(name string) (string, error) {
	return r.run("workspace", "delete", name)
}

// Outputs returns terraform outputs.
func (r *Runner) Outputs() ([]Output, error) {
	out, err := r.run("output", "-json")
	if err != nil {
		if strings.Contains(out, "no outputs") || strings.Contains(out, "No outputs") {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", out, err)
	}

	var raw map[string]struct {
		Value     any  `json:"value"`
		Type      any  `json:"type"`
		Sensitive bool `json:"sensitive"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, err
	}

	var outputs []Output
	for name, o := range raw {
		typeStr := fmt.Sprintf("%v", o.Type)
		outputs = append(outputs, Output{
			Name:      name,
			Value:     o.Value,
			Type:      typeStr,
			Sensitive: o.Sensitive,
		})
	}
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].Name < outputs[j].Name
	})
	return outputs, nil
}

// ListFiles returns .tf and .tfvars files recursively, skipping hidden
// dirs and .terraform/ caches. Files are sorted by relative path.
func (r *Runner) ListFiles() ([]TfFile, error) {
	var files []TfFile

	err := filepath.Walk(r.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		name := info.Name()

		// Skip hidden directories and .terraform caches
		if info.IsDir() {
			if name == ".terraform" || name == ".git" || (len(name) > 1 && name[0] == '.') {
				return filepath.SkipDir
			}
			return nil
		}

		isTf := strings.HasSuffix(name, ".tf")
		isVars := strings.HasSuffix(name, ".tfvars") || strings.HasSuffix(name, ".tfvars.json")
		if !isTf && !isVars {
			return nil
		}

		rel, _ := filepath.Rel(r.WorkDir, path)
		dir := filepath.Dir(rel)
		if dir == "." {
			dir = ""
		}
		depth := 0
		if dir != "" {
			depth = strings.Count(dir, string(filepath.Separator)) + 1
		}

		files = append(files, TfFile{
			Name:    name,
			Path:    path,
			RelPath: rel,
			Dir:     dir,
			IsVars:  isVars,
			Size:    info.Size(),
			Depth:   depth,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	return files, nil
}

// DirTree represents a directory node for tree rendering.
type DirTree struct {
	Name     string
	RelPath  string
	Children []*DirTree
	Files    []TfFile
}

// BuildFileTree organizes flat file list into a directory tree.
func BuildFileTree(files []TfFile) *DirTree {
	root := &DirTree{Name: ".", RelPath: ""}
	dirMap := map[string]*DirTree{"": root}

	// Ensure all parent directories exist
	ensureDir := func(dirPath string) *DirTree {
		if dirPath == "" {
			return root
		}
		if d, ok := dirMap[dirPath]; ok {
			return d
		}
		// Build path segment by segment
		parts := strings.Split(dirPath, string(filepath.Separator))
		current := root
		built := ""
		for _, part := range parts {
			if built == "" {
				built = part
			} else {
				built = built + string(filepath.Separator) + part
			}
			if d, ok := dirMap[built]; ok {
				current = d
			} else {
				newDir := &DirTree{Name: part, RelPath: built}
				current.Children = append(current.Children, newDir)
				dirMap[built] = newDir
				current = newDir
			}
		}
		return current
	}

	for _, f := range files {
		dir := ensureDir(f.Dir)
		dir.Files = append(dir.Files, f)
	}

	return root
}

// ReadFile reads the content of a file.
func (r *Runner) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Providers returns the providers in use.
func (r *Runner) Providers() (string, error) {
	return r.run("providers")
}

// Graph returns the dependency graph in DOT format.
func (r *Runner) Graph() (string, error) {
	return r.run("graph")
}

// parseResourceAddress parses a terraform resource address like
// module.foo.aws_instance.bar into a Resource struct.
func parseResourceAddress(addr string) Resource {
	res := Resource{Address: addr}
	parts := strings.Split(addr, ".")

	if strings.HasPrefix(addr, "module.") {
		// module.name.type.name or module.name.module.name2.type.name
		modParts := []string{}
		i := 0
		for i < len(parts)-2 {
			if parts[i] == "module" && i+1 < len(parts) {
				modParts = append(modParts, parts[i+1])
				i += 2
			} else {
				break
			}
		}
		res.Module = strings.Join(modParts, ".")
		if i < len(parts)-1 {
			res.Type = parts[i]
			res.Name = parts[i+1]
		} else if i < len(parts) {
			res.Type = parts[i]
		}
	} else if len(parts) >= 2 {
		// type.name (e.g., aws_instance.example)
		res.Type = parts[0]
		res.Name = strings.Join(parts[1:], ".")
	} else {
		res.Type = addr
	}

	return res
}

// IsInitialized checks if terraform has been initialized.
func (r *Runner) IsInitialized() bool {
	_, err := os.Stat(filepath.Join(r.WorkDir, ".terraform"))
	return err == nil
}

// Version returns the terraform version.
func (r *Runner) Version() string {
	out, err := r.run("version", "-json")
	if err != nil {
		return r.Binary
	}
	var v struct {
		Version string `json:"terraform_version"`
	}
	if json.Unmarshal([]byte(out), &v) == nil && v.Version != "" {
		return fmt.Sprintf("%s v%s", r.Binary, v.Version)
	}
	// fallback
	out, _ = r.run("version")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return r.Binary
}
