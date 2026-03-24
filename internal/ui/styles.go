package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color palette — inspired by lazygit's purple/magenta theme
// with terraform-specific accent colors.
var (
	// Core colors
	Purple     = lipgloss.Color("#7B61FF")
	Magenta    = lipgloss.Color("#FF6EC7")
	Cyan       = lipgloss.Color("#00D4AA")
	Green      = lipgloss.Color("#00E676")
	Yellow     = lipgloss.Color("#FFD600")
	Red        = lipgloss.Color("#FF5252")
	Orange     = lipgloss.Color("#FF9100")
	Blue       = lipgloss.Color("#448AFF")
	DimGray    = lipgloss.Color("#555555")
	MediumGray = lipgloss.Color("#888888")
	LightGray  = lipgloss.Color("#BBBBBB")
	White      = lipgloss.Color("#FFFFFF")
	BgDark     = lipgloss.Color("#1A1A2E")
	BgPanel    = lipgloss.Color("#16213E")
	BgActive   = lipgloss.Color("#0F3460")

	// Panel styles
	ActivePanelBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(Purple)

	InactivePanelBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(DimGray)

	// Title styles
	PanelTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(White).
			Background(Purple).
			Padding(0, 1)

	InactivePanelTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(LightGray).
				Background(DimGray).
				Padding(0, 1)

	// List item styles
	SelectedItem = lipgloss.NewStyle().
			Foreground(White).
			Background(BgActive).
			Bold(true)

	NormalItem = lipgloss.NewStyle().
			Foreground(LightGray)

	DimItem = lipgloss.NewStyle().
		Foreground(MediumGray)

	// Resource type colors
	ResourceType = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	ResourceName = lipgloss.NewStyle().
			Foreground(White)

	ModuleName = lipgloss.NewStyle().
			Foreground(Yellow)

	// Status bar
	StatusBar = lipgloss.NewStyle().
			Foreground(White).
			Background(lipgloss.Color("#2D2D44")).
			Padding(0, 1)

	StatusKey = lipgloss.NewStyle().
			Foreground(Purple).
			Bold(true)

	StatusValue = lipgloss.NewStyle().
			Foreground(LightGray)

	// Git branch
	GitBranchIcon = lipgloss.NewStyle().
			Foreground(Magenta).
			Bold(true)

	GitBranchName = lipgloss.NewStyle().
			Foreground(Cyan)

	// Help text
	HelpKey = lipgloss.NewStyle().
		Foreground(Purple).
		Bold(true)

	HelpDesc = lipgloss.NewStyle().
			Foreground(MediumGray)

	HelpSep = lipgloss.NewStyle().
		Foreground(DimGray)

	// Plan output highlighting
	PlanAdd = lipgloss.NewStyle().
		Foreground(Green)

	PlanChange = lipgloss.NewStyle().
			Foreground(Yellow)

	PlanDestroy = lipgloss.NewStyle().
			Foreground(Red)

	PlanInfo = lipgloss.NewStyle().
		Foreground(Cyan)

	// Section header in sidebar
	SectionHeader = lipgloss.NewStyle().
			Foreground(Purple).
			Bold(true).
			MarginTop(1)

	// Error/warning/success
	ErrorStyle = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true)

	WarningStyle = lipgloss.NewStyle().
			Foreground(Yellow)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Green).
			Bold(true)

	// Spinner label
	SpinnerLabel = lipgloss.NewStyle().
			Foreground(Purple).
			Bold(true)

	// Logo/brand
	Logo = lipgloss.NewStyle().
		Foreground(Purple).
		Bold(true)
)

// HighlightPlanLine applies color to a terraform plan output line.
// Deprecated: Use PlanHighlighter.HighlightLine instead, which is
// heredoc-aware and won't miscolor YAML list items as removals.
func HighlightPlanLine(line string) string {
	trimmed := line
	for len(trimmed) > 0 && trimmed[0] == ' ' {
		trimmed = trimmed[1:]
	}

	switch {
	case len(trimmed) > 0 && trimmed[0] == '+':
		return PlanAdd.Render(line)
	case len(trimmed) > 0 && trimmed[0] == '-':
		return PlanDestroy.Render(line)
	case len(trimmed) > 0 && trimmed[0] == '~':
		return PlanChange.Render(line)
	case strings.Contains(line, "Plan:"):
		return PlanInfo.Render(line)
	case strings.Contains(line, "No changes"):
		return SuccessStyle.Render(line)
	case strings.Contains(line, "Error"):
		return ErrorStyle.Render(line)
	case strings.Contains(line, "Warning"):
		return WarningStyle.Render(line)
	default:
		return line
	}
}


