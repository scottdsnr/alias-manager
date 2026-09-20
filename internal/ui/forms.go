package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/scotthellings/alias-manager/internal/aliasfile"
	"github.com/scotthellings/alias-manager/internal/config"
)

// ---------- alias form ----------

type aliasForm struct {
	inputs  []textinput.Model // name, command, comment
	focus   int
	groups  []string
	groupIx int
	oldName string
	dupOf   string
	enabled bool
	err     string
}

const (
	fName = iota
	fCommand
	fComment
	fGroup // pseudo-field: selected with left/right
	fieldCount
)

func (m *Model) openAliasForm(n *aliasfile.Node) (tea.Model, tea.Cmd) {
	return m.openAliasFormWith(n, false)
}

// openDuplicateForm opens the edit screen pre-filled from n but detached from
// it, so submitting creates a second alias instead of renaming the original.
func (m *Model) openDuplicateForm(n *aliasfile.Node) (tea.Model, tea.Cmd) {
	return m.openAliasFormWith(n, true)
}

func (m *Model) openAliasFormWith(n *aliasfile.Node, dup bool) (tea.Model, tea.Cmd) {
	f := aliasForm{groups: m.groupNames(), enabled: true}
	mk := func(placeholder, val string, limit int) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.SetValue(val)
		t.CharLimit = limit
		t.Width = 56
		return t
	}
	if n != nil {
		name := n.Name
		if dup {
			f.dupOf = n.Name
			name = m.copyName(n.Name)
		} else {
			f.oldName = n.Name
		}
		f.enabled = n.Enabled
		f.inputs = []textinput.Model{
			mk("gs", name, 64),
			mk("git status", n.Command, 1024),
			mk("optional note", n.Comment, 128),
		}
		for i, g := range f.groups {
			if g == n.Group {
				f.groupIx = i
			}
		}
	} else {
		f.inputs = []textinput.Model{mk("gs", "", 64), mk("git status", "", 1024), mk("optional note", "", 128)}
		if r := m.currentRow(); r != nil {
			for i, g := range f.groups {
				if g == r.group {
					f.groupIx = i
				}
			}
		}
	}
	f.inputs[0].Focus()
	f.inputs[0].CursorEnd()
	m.alias = f
	m.screen = screenAlias
	return m, textinput.Blink
}

func (m *Model) updateAliasForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.alias
	switch msg.String() {
	case "esc":
		m.screen = screenList
		return m, nil
	case "tab", "down":
		f.focus = (f.focus + 1) % fieldCount
	case "shift+tab", "up":
		f.focus = (f.focus - 1 + fieldCount) % fieldCount
	case "ctrl+e":
		f.enabled = !f.enabled
	case "left":
		if f.focus == fGroup && len(f.groups) > 0 {
			f.groupIx = (f.groupIx - 1 + len(f.groups)) % len(f.groups)
		}
	case "right":
		if f.focus == fGroup && len(f.groups) > 0 {
			f.groupIx = (f.groupIx + 1) % len(f.groups)
		}
	case "enter":
		return m.submitAlias()
	}
	for i := range f.inputs {
		if i == f.focus {
			f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
	if f.focus < len(f.inputs) {
		var cmd tea.Cmd
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) submitAlias() (tea.Model, tea.Cmd) {
	f := &m.alias
	group := aliasfile.Ungrouped
	if len(f.groups) > 0 {
		group = f.groups[f.groupIx]
	}
	n := aliasfile.Node{
		Name:    strings.TrimSpace(f.inputs[fName].Value()),
		Command: strings.TrimSpace(f.inputs[fCommand].Value()),
		Comment: strings.TrimSpace(f.inputs[fComment].Value()),
		Enabled: f.enabled,
		Group:   group,
	}
	if err := m.doc.Upsert(f.oldName, n); err != nil {
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
	return m, m.save(fmt.Sprintf("%s %s", verb, n.Name))
}

// copyName suggests a free name for a duplicate: gs -> gs-copy, gs-copy-2, ...
func (m *Model) copyName(name string) string {
	taken := map[string]bool{}
	for _, n := range m.doc.Nodes {
		if n.Kind == aliasfile.KindAlias {
			taken[n.Name] = true
		}
	}
	cand := name + "-copy"
	for i := 2; taken[cand]; i++ {
		cand = fmt.Sprintf("%s-copy-%d", name, i)
	}
	return cand
}

// ---------- group form ----------

type groupForm struct {
	input textinput.Model
	old   string
	err   string
}

func (m *Model) openGroupForm(old string) (tea.Model, tea.Cmd) {
	t := textinput.New()
	t.Placeholder = "Git"
	t.CharLimit = 48
	t.Width = 40
	if old != "" && old != aliasfile.Ungrouped {
		t.SetValue(old)
	} else {
		old = ""
	}
	t.Focus()
	m.group = groupForm{input: t, old: old}
	m.screen = screenGroup
	return m, textinput.Blink
}

func (m *Model) updateGroupForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	g := &m.group
	switch msg.String() {
	case "esc":
		m.screen = screenList
		return m, nil
	case "enter":
		name := strings.TrimSpace(g.input.Value())
		var err error
		if g.old == "" {
			err = m.doc.AddGroup(name)
		} else {
			err = m.doc.RenameGroup(g.old, name)
		}
		if err != nil {
			g.err = err.Error()
			return m, nil
		}
		m.screen = screenList
		m.rebuild()
		return m, m.save("group saved: " + name)
	}
	var cmd tea.Cmd
	g.input, cmd = g.input.Update(msg)
	return m, cmd
}

// ---------- confirm ----------

type confirmPrompt struct {
	target      row
	withAliases bool
}

func (m *Model) openConfirm(r row) (tea.Model, tea.Cmd) {
	m.confirm = confirmPrompt{target: r}
	m.screen = screenConfirm
	return m, nil
}

func (m *Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.confirm
	switch msg.String() {
	case "esc", "n", "q":
		m.screen = screenList
		return m, nil
	case "tab", " ":
		if c.target.kind == rowGroup {
			c.withAliases = !c.withAliases
		}
		return m, nil
	case "y", "enter":
		var note string
		if c.target.kind == rowAlias {
			m.doc.Delete(c.target.node.Name)
			note = "deleted " + c.target.node.Name
		} else {
			if c.target.group == aliasfile.Ungrouped {
				m.screen = screenList
				m.err = "the Ungrouped bucket cannot be deleted"
				return m, nil
			}
			m.doc.DeleteGroup(c.target.group, c.withAliases)
			note = "deleted group " + c.target.group
			if c.withAliases {
				note += " and its aliases"
			}
		}
		m.screen = screenList
		m.rebuild()
		return m, m.save(note)
	}
	return m, nil
}

// ---------- settings ----------

type settingsForm struct {
	inputs  []textinput.Model // alias file, source file
	focus   int
	shell   string
	color   string
	first   bool
	suggest []string
	suggIx  int
	err     string
}

const (
	sAliasFile = iota
	sSourceFile
	sShell
	sColor
	sFieldCount
)

func newSettingsForm(cfg *config.Config, first bool) settingsForm {
	mk := func(ph, val string) textinput.Model {
		t := textinput.New()
		t.Placeholder = ph
		t.SetValue(val)
		t.CharLimit = 512
		t.Width = 56
		return t
	}
	f := settingsForm{
		inputs:  []textinput.Model{mk("~/.bash_aliases", cfg.AliasFile), mk("leave blank to use the alias file", cfg.SourceFile)},
		shell:   cfg.Shell,
		color:   cfg.Color,
		first:   first,
		suggest: config.Candidates(),
	}
	if f.shell == "" {
		f.shell = config.DetectShell()
	}
	if !ValidColor(f.color) {
		f.color = DefaultColor
	}
	f.inputs[0].Focus()
	return f
}

func (m *Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.settings
	switch msg.String() {
	case "esc":
		if f.first {
			m.quit = true
			return m, tea.Quit
		}
		// Undo any live colour preview that was never saved.
		applyColor(m.cfg.Color)
		m.screen = screenList
		return m, nil
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "tab", "down":
		f.focus = (f.focus + 1) % sFieldCount
	case "shift+tab", "up":
		f.focus = (f.focus - 1 + sFieldCount) % sFieldCount
	case "left", "right":
		if f.focus == sShell {
			if f.shell == "zsh" {
				f.shell = "bash"
			} else {
				f.shell = "zsh"
			}
			return m, nil
		}
		if f.focus == sColor {
			step := 1
			if msg.String() == "left" {
				step = -1
			}
			f.color = Colors[(indexOf(Colors, f.color)+step+len(Colors))%len(Colors)]
			// Preview immediately so the choice is visible while cycling.
			applyColor(f.color)
			return m, nil
		}
	case "ctrl+n":
		// Cycle the detected candidate files into the alias-file field.
		if len(f.suggest) > 0 {
			f.inputs[sAliasFile].SetValue(f.suggest[f.suggIx])
			f.suggIx = (f.suggIx + 1) % len(f.suggest)
			return m, nil
		}
	case "enter":
		return m.submitSettings()
	}
	for i := range f.inputs {
		if i == f.focus {
			f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
	if f.focus < len(f.inputs) {
		var cmd tea.Cmd
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) submitSettings() (tea.Model, tea.Cmd) {
	f := &m.settings
	path := config.ExpandPath(f.inputs[sAliasFile].Value())
	if path == "" {
		f.err = "enter the path to the file your aliases live in"
		return m, nil
	}
	if err := ensureWritable(path); err != nil {
		f.err = err.Error()
		return m, nil
	}
	src := config.ExpandPath(f.inputs[sSourceFile].Value())
	if src == "" {
		src = path
	}
	m.cfg.AliasFile, m.cfg.SourceFile, m.cfg.Shell, m.cfg.Color = path, src, f.shell, f.color
	applyColor(f.color)
	if err := m.cfg.Save(); err != nil {
		f.err = err.Error()
		return m, nil
	}
	if err := m.reload(); err != nil {
		f.err = err.Error()
		return m, nil
	}
	f.first = false
	m.screen = screenList
	m.status = "settings saved · using " + short(path)
	return m, nil
}

// ---------- move: destination group picker ----------

type moveForm struct {
	groups []string
	ix     int
	count  int
	err    string
}

func (m *Model) openMoveForm() (tea.Model, tea.Cmd) {
	f := moveForm{groups: m.groupNames(), count: len(m.selected)}
	// Start on the group the cursor is sitting in, if it is a real choice.
	if r := m.currentRow(); r != nil {
		for i, g := range f.groups {
			if g == r.group {
				f.ix = i
			}
		}
	}
	m.move = f
	m.screen = screenMove
	return m, nil
}

func (m *Model) updateMoveForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.move
	switch msg.String() {
	case "esc":
		// Back to move mode with the selection intact.
		m.screen = screenList
		return m, nil
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up", "k", "left":
		f.ix = (f.ix - 1 + len(f.groups)) % len(f.groups)
	case "down", "j", "right":
		f.ix = (f.ix + 1) % len(f.groups)
	case "N":
		// Make a new group, then land the selection in it.
		m.screen = screenList
		return m.openGroupForm("")
	case "enter":
		if len(f.groups) == 0 {
			return m, nil
		}
		group := f.groups[f.ix]
		names := m.selectedNames()
		if err := m.doc.MoveToGroup(names, group); err != nil {
			f.err = err.Error()
			return m, nil
		}
		m.moving = false
		m.selected = map[string]bool{}
		m.screen = screenList
		m.rebuild()
		return m, m.save(fmt.Sprintf("moved %d alias(es) to %s", len(names), group))
	}
	return m, nil
}
