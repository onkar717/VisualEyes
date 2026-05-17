package styles

import "github.com/charmbracelet/lipgloss"

var (
	Cyan   = lipgloss.Color("#00D7FF")
	Green  = lipgloss.Color("#00FF87")
	Gold   = lipgloss.Color("#FFD700")
	Red    = lipgloss.Color("#FF5F5F")
	Orange = lipgloss.Color("#FF8C00")
	Gray   = lipgloss.Color("#626262")
	White  = lipgloss.Color("#FFFFFF")
	Dim    = lipgloss.Color("#4A4A4A")

	Banner = lipgloss.NewStyle().
		Foreground(Cyan).
		Bold(true)

	Title = lipgloss.NewStyle().
		Foreground(White).
		Bold(true).
		MarginBottom(1)

	SectionHeader = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true).
			MarginTop(1)

	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#303060")).
		Padding(0, 1)

	KeyStyle = lipgloss.NewStyle().Foreground(Gray)
	ValStyle = lipgloss.NewStyle().Foreground(White)

	SevCritical = lipgloss.NewStyle().Foreground(Red).Bold(true)
	SevWarning  = lipgloss.NewStyle().Foreground(Gold).Bold(true)
	SevInfo     = lipgloss.NewStyle().Foreground(Cyan)

	StatusFiring   = lipgloss.NewStyle().Foreground(Red)
	StatusResolved = lipgloss.NewStyle().Foreground(Green)

	Good = lipgloss.NewStyle().Foreground(Green)
	Bad  = lipgloss.NewStyle().Foreground(Red)
	Mute = lipgloss.NewStyle().Foreground(Gray)

	HelpBar = lipgloss.NewStyle().
		Foreground(Gray).
		MarginTop(1)

	Selected = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)
)

func Severity(sev string) lipgloss.Style {
	switch sev {
	case "critical":
		return SevCritical
	case "warning":
		return SevWarning
	default:
		return SevInfo
	}
}

func SeverityBadge(sev string) string {
	switch sev {
	case "critical":
		return SevCritical.Render("[CRIT]")
	case "warning":
		return SevWarning.Render("[WARN]")
	default:
		return SevInfo.Render("[INFO]")
	}
}
