package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary = lipgloss.Color("#5AF78E")
	colorAccent  = lipgloss.Color("#57C7FF")
	colorWarn    = lipgloss.Color("#F3C969")
	colorError   = lipgloss.Color("#FF5C57")
	colorDim     = lipgloss.Color("#9AA4B2")
	colorBorder  = lipgloss.Color("#6C7A89")
	colorCursor  = lipgloss.Color("#25384A")
	colorText    = lipgloss.Color("#EEEEEE")

	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	activeTabStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Underline(true)
	dimStyle          = lipgloss.NewStyle().Foreground(colorDim)
	keyStyle          = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	actionStyle       = lipgloss.NewStyle().Foreground(colorPrimary)
	bulletStyle       = lipgloss.NewStyle().Foreground(colorDim)
	accentStyle       = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	currentStyle      = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	warnStyle         = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	errorTextStyle    = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	statusStyle       = lipgloss.NewStyle().Foreground(colorAccent)
	panelStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder).Padding(0, 1)
	panelActiveStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1)
	panelHeaderStyle  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	selectedItemStyle = lipgloss.NewStyle().Foreground(colorText).Background(colorCursor).Bold(true)
	labelStyle        = lipgloss.NewStyle().Foreground(colorAccent).Width(8)
	detailStyle       = lipgloss.NewStyle().Foreground(colorText)
	helpStyle         = lipgloss.NewStyle().Padding(1, 2)
)
