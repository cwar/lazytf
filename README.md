# ⚡ lazytf

A **lazygit-style TUI** for Terraform. Navigate your infrastructure, run plans, manage state, and switch workspaces — all from a beautiful terminal interface.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)

## Features

- 📄 **File Browser** — Browse and view `.tf` and `.tfvars` files
- 🏗 **Resource Explorer** — List and inspect resources in state
- 📁 **Workspace Manager** — Switch between workspaces
- ⚙ **Var-file Selector** — Toggle var-files for plan/apply
- 📤 **Output Viewer** — View terraform outputs
- 🎨 **Syntax Highlighting** — Color-coded plan output (adds/changes/destroys)
- 📋 **Command Log** — Full history of terraform commands run
- 🔒 **Confirm Dialogs** — Safety prompts for apply and destroy
- 🦎 **OpenTofu Support** — Auto-detects `tofu` vs `terraform`

## Install

```bash
go install github.com/cwar/lazytf/cmd/lazytf@latest
```

Or build from source:

```bash
git clone https://github.com/cwar/lazytf.git
cd lazytf
go build -o lazytf ./cmd/lazytf/
```

## Usage

```bash
# In a terraform directory
lazytf

# Or specify a path
lazytf /path/to/terraform/project
```

## Keyboard Shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `j`/`k`, `↑`/`↓` | Move up/down |
| `Tab` | Switch between sidebar and main pane |
| `Enter`/`Space` | Select item / Toggle section |
| `g`/`G` | Go to top/bottom (main pane) |
| `d`/`u` | Page down/up (main pane) |

### Terraform Commands

| Key | Action |
|-----|--------|
| `p` | Run `terraform plan` |
| `a` | Run `terraform apply` (with confirmation) |
| `i` | Run `terraform init` |
| `v` | Run `terraform validate` |
| `f` | Run `terraform fmt -check` |
| `F` | Run `terraform fmt` (fix) |
| `D` | Run `terraform destroy` (with confirmation) |
| `P` | Show providers |

### Multi-Workspace Operations

| Key | Action |
|-----|--------|
| `W` | Multi-workspace plan (parallel across workspaces) |
| `j`/`k` | Select workspace (in multi-ws mode) |
| `y` | Apply selected workspace |
| `A` | Apply ALL workspaces with changes (sequential) |
| `esc` | Close / cancel all |

Press `W` to enter multi-workspace mode. You'll be prompted for a filter (e.g. "dev", "prod") or leave blank for all workspaces. Plans run in parallel (up to 4 concurrent). Review results, then apply individually or all at once.

### State Management

| Key | Action |
|-----|--------|
| `t` | Taint selected resource |
| `u` | Untaint selected resource |

### General

| Key | Action |
|-----|--------|
| `r` | Refresh data |
| `l` | Toggle command log |
| `?` | Toggle help overlay |
| `q`/`Ctrl+C` | Quit |

## How It Works

lazytf wraps the `terraform` CLI with a terminal UI built on [Bubbletea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss). It reads your terraform project structure, state, and workspaces to present an interactive explorer.

### Layout

```
┌─ Explorer ──────┐┌─ main.tf ──────────────────────┐
│ ▾ 📄 Files (5)  ││ terraform {                     │
│   main.tf       ││   required_providers {           │
│   variables.tf  ││     aws = {                      │
│   outputs.tf    ││       source  = "hashicorp/aws"  │
│ ▾ 🏗 Resources  ││     }                            │
│   aws_vpc.main  ││   }                              │
│ ▾ 📁 Workspaces ││ }                                │
│ ● default       ││                                  │
│ ▾ ⚙ Var Files   ││                                  │
│ ● prod.tfvars   ││                                  │
└─────────────────┘└──────────────────────────────────┘
 workspace: default                    ✓ Plan complete
 p:plan │ a:apply │ i:init │ ?:help │ q:quit
```

## Configuration

Create a `.lazytf.yaml` in your terraform project directory to customize behavior:

```yaml
# Workspaces to skip during multi-workspace operations (W key).
# These still appear in the normal workspace panel.
ignore_workspaces:
  - default
  - prod-gae2

# Named groups for quick workspace filtering.
# When you press W and type "dev", it resolves to the filter "dev-".
workspace_groups:
  dev: dev-
  prod: prod-
  podcast: podcast
  osd: osd
```

### Workspace Groups

Groups let you define shorthand names for workspace filters. When entering multi-workspace mode (`W`), typing a group name applies its filter pattern. If the input doesn't match a group name, it's used as a plain substring filter.

## License

MIT
