package ui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent = lipgloss.Color("81")
	colMuted  = lipgloss.Color("244")
	colWarn   = lipgloss.Color("203")
	colOK     = lipgloss.Color("78")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(colAccent).Padding(0, 1)
	groupStyle    = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	nameStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
	cmdStyle      = lipgloss.NewStyle().Foreground(colMuted)
	disabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Strikethrough(true)
	cursorStyle   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	helpStyle     = lipgloss.NewStyle().Foreground(colMuted)
	errStyle      = lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	okStyle       = lipgloss.NewStyle().Foreground(colOK)
	labelStyle    = lipgloss.NewStyle().Foreground(colMuted).Width(10)
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
)
