package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/scotthellings/alias-manager/internal/aliasfile"
)

// bodyIndent is what the tab key inserts inside the function body. Shell
// functions are indented with spaces here so the file reads the same
// everywhere, whatever a terminal's tab width happens to be.
const bodyIndent = "    "

type funcForm struct {
	name    textinput.Model
	body    textarea.Model
	comment textinput.Model
	focus   int
	groups  []string
	groupIx int
	oldName string
	dupOf   string
	enabled bool
	err     string
}

const (
	gName = iota
	gBody
	gComment
	gGroup // pseudo-field: selected with left/right
	gFieldCount
)

func (m *Model) openFuncForm(n *aliasfile.Node) (tea.Model, tea.Cmd) {
	return m.openFuncFormWith(n, false)
}

// openDuplicateFuncForm pre-fills from n but detaches from it, so submitting
// creates a second function instead of renaming the original.
func (m *Model) openDuplicateFuncForm(n *aliasfile.Node) (tea.Model, tea.Cmd) {
	return m.openFuncFormWith(n, true)
}

func (m *Model) openFuncFormWith(n *aliasfile.Node, dup bool) (tea.Model, tea.Cmd) {
	f := funcForm{groups: m.groupNames(), enabled: true}

	f.name = textinput.New()
	f.name.Placeholder = "mkcd"
	f.name.CharLimit = 64
	f.name.Width = 56

	f.comment = textinput.New()
	f.comment.Placeholder = "optional note"
	f.comment.CharLimit = 128
	f.comment.Width = 56

	f.body = textarea.New()
	f.body.Placeholder = "mkdir -p \"$1\" && cd \"$1\""
	f.body.CharLimit = 0
	f.body.ShowLineNumbers = true
	f.body.SetWidth(58)
	lines := 1
	if n != nil {
		lines = strings.Count(n.Body, "\n") + 1
	}
	f.body.SetHeight(m.bodyHeight(lines))
	f.body.Prompt = "│ "

	if n != nil {
		name := n.Name
		if dup {
			f.dupOf = n.Name
			name = m.copyFuncName(n.Name)
		} else {
			f.oldName = n.Name
		}
		f.name.SetValue(name)
		f.comment.SetValue(n.Comment)
		f.body.SetValue(n.Body)
		f.enabled = n.Enabled
		for i, g := range f.groups {
			if g == n.Group {
				f.groupIx = i
			}
		}
	} else {
		f.body.SetValue(bodyIndent)
		if r := m.currentRow(); r != nil {
			for i, g := range f.groups {
				if g == r.group {
					f.groupIx = i
				}
			}
		}
	}
	f.name.Focus()
	f.name.CursorEnd()
	m.fn = f
	m.screen = screenFunc
	return m, textinput.Blink
}

// bodyHeight sizes the editor to the function it is holding, with room to
// grow, but never past what the terminal can show alongside the other fields.
func (m *Model) bodyHeight(lines int) int {
	h := lines + 3
	if room := m.height - 16; room > 4 && h > room {
		h = room
	}
	if h < 5 {
		h = 5
	}
	if h > 20 {
		h = 20
	}
	return h
}

func (m *Model) updateFuncForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.fn
	switch msg.String() {
	case "esc":
		m.screen = screenList
		return m, nil
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "ctrl+e":
		f.enabled = !f.enabled
		return m, nil
	case "ctrl+s":
		return m.submitFunc()
	case "tab":
		// Inside the body, tab is indentation; everywhere else it moves on.
		if f.focus == gBody {
			f.body.InsertString(bodyIndent)
			return m, nil
		}
		f.focus = (f.focus + 1) % gFieldCount
	case "ctrl+n":
		f.focus = (f.focus + 1) % gFieldCount
	case "shift+tab", "ctrl+p":
		f.focus = (f.focus - 1 + gFieldCount) % gFieldCount
	case "down":
		// The body owns the arrow keys so the cursor can move through it.
		if f.focus != gBody {
			f.focus = (f.focus + 1) % gFieldCount
		}
	case "up":
		if f.focus != gBody {
			f.focus = (f.focus - 1 + gFieldCount) % gFieldCount
		}
	case "left":
		if f.focus == gGroup && len(f.groups) > 0 {
			f.groupIx = (f.groupIx - 1 + len(f.groups)) % len(f.groups)
		}
	case "right":
		if f.focus == gGroup && len(f.groups) > 0 {
			f.groupIx = (f.groupIx + 1) % len(f.groups)
		}
	case "enter":
		// In the body, enter is a newline; elsewhere it saves.
		if f.focus != gBody {
			return m.submitFunc()
		}
	}
	return m, f.route(msg)
}

// route focuses the selected field and hands the key to it.
func (f *funcForm) route(msg tea.KeyMsg) tea.Cmd {
	f.name.Blur()
	f.comment.Blur()
	f.body.Blur()
	var cmd tea.Cmd
	switch f.focus {
	case gName:
		f.name.Focus()
		f.name, cmd = f.name.Update(msg)
	case gComment:
		f.comment.Focus()
		f.comment, cmd = f.comment.Update(msg)
	case gBody:
		f.body.Focus()
		f.body, cmd = f.body.Update(msg)
	}
	return cmd
}

func (m *Model) submitFunc() (tea.Model, tea.Cmd) {
	f := &m.fn
	group := aliasfile.Ungrouped
	if len(f.groups) > 0 {
		group = f.groups[f.groupIx]
	}
	n := aliasfile.Node{
		Name:    strings.TrimSpace(f.name.Value()),
		Body:    normalizeBody(f.body.Value()),
		Comment: strings.TrimSpace(f.comment.Value()),
		Enabled: f.enabled,
		Group:   group,
	}
	if err := m.doc.UpsertFunc(f.oldName, n); err != nil {
		f.err = err.Error()
		return m, nil
	}
	verb := "created"
	switch {
	case f.oldName != "":
		verb = "updated"
	case f.dupOf != "":
		verb = "duplicated " + f.dupOf + " as"
	}
	m.screen = screenList
	m.rebuild()
	return m, m.save(fmt.Sprintf("%s %s()", verb, n.Name))
}

// normalizeBody trims blank lines off the ends and turns any stray tab into
// spaces, so the written function is never indented inconsistently.
func normalizeBody(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\t", bodyIndent), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// copyFuncName suggests a free name for a duplicate: gs -> gs_copy.
func (m *Model) copyFuncName(name string) string {
	taken := map[string]bool{}
	for _, n := range m.doc.Nodes {
		if n.Kind == aliasfile.KindFunc {
			taken[n.Name] = true
		}
	}
	cand := name + "_copy"
	for i := 2; taken[cand]; i++ {
		cand = fmt.Sprintf("%s_copy_%d", name, i)
	}
	return cand
}
