package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPurple  = lipgloss.Color("99")
	colorGreen   = lipgloss.Color("46")
	colorOrange  = lipgloss.Color("208")
	colorRed     = lipgloss.Color("196")
	colorBlue    = lipgloss.Color("39")
	colorDim     = lipgloss.Color("240")
	colorNormal  = lipgloss.Color("252")
	colorYellow  = lipgloss.Color("226")
	colorBg      = lipgloss.Color("235")
	colorSelected = lipgloss.Color("57")
	colorSelFg   = lipgloss.Color("230")

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPurple).
			Background(colorBg).
			Padding(0, 1)

	styleFooter = lipgloss.NewStyle().
			Foreground(colorDim).
			Background(colorBg).
			Padding(0, 1)

	styleSelected = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorSelFg)

	styleNormal = lipgloss.NewStyle().
			Foreground(colorNormal)

	styleDim = lipgloss.NewStyle().
			Foreground(colorDim)

	styleBold = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorNormal)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPurple)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorGreen)

	styleError = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	styleWarn = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true)

	styleInfo = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	styleDebug = lipgloss.NewStyle().
			Foreground(colorDim).
			Bold(true)

	styleOverlay = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPurple).
			Background(colorBg).
			Padding(1, 2)
)

func levelStyle(level string) lipgloss.Style {
	switch level {
	case "ERROR":
		return styleError
	case "WARN":
		return styleWarn
	case "INFO":
		return styleInfo
	case "DEBUG":
		return styleDebug
	default:
		return styleNormal
	}
}
