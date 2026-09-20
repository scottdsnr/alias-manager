package ui

import "github.com/charmbracelet/lipgloss"

// Colors are the accent colours offered in settings, in cycle order.
var Colors = []string{"teal", "green", "magenta", "yellow", "purple"}

var accents = map[string]lipgloss.Color{
	"teal":    lipgloss.Color("81"),
	"green":   lipgloss.Color("78"),
	"magenta": lipgloss.Color("205"),
	"yellow":  lipgloss.Color("220"),
	"purple":  lipgloss.Color("141"),
}

// DefaultColor is used when the config names no colour, or an unknown one.
const DefaultColor = "teal"

var (
	colAccent = accents[DefaultColor]
	colMuted  = lipgloss.Color("244")
	colWarn   = lipgloss.Color("203")
	colOK     = lipgloss.Color("78")

	titleStyle    lipgloss.Style
	groupStyle    lipgloss.Style
	nameStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
	cmdStyle      = lipgloss.NewStyle().Foreground(colMuted)
	disabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Strikethrough(true)
	cursorStyle   lipgloss.Style
	helpStyle     = lipgloss.NewStyle().Foreground(colMuted)
	errStyle      = lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	okStyle       = lipgloss.NewStyle().Foreground(colOK)
	labelStyle    = lipgloss.NewStyle().Foreground(colMuted).Width(10)
	boxStyle      lipgloss.Style
)

func init() { applyColor(DefaultColor) }

// applyColor repoints every accent-tinted style at the named colour. Unknown
// names fall back to the default rather than leaving the UI unstyled.
func applyColor(name string) {
	c, ok := accents[name]
	if !ok {
		c = accents[DefaultColor]
	}
	colAccent = c
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(colAccent).Padding(0, 1)
	groupStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	cursorStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	boxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
}

// ValidColor reports whether name is one of the offered colours.
func ValidColor(name string) bool { _, ok := accents[name]; return ok }

func indexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return 0
}
