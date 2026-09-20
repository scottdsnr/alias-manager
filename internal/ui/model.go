package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/scotthellings/alias-manager/internal/aliasfile"
	"github.com/scotthellings/alias-manager/internal/config"
	"github.com/scotthellings/alias-manager/internal/shell"
)

type screen int

const (
	screenList screen = iota
	screenAlias
	screenGroup
	screenSettings
	screenConfirm
	screenHelp
)

type rowKind int

const (
	rowGroup rowKind = iota
	rowAlias
)

type row struct {
	kind  rowKind
	group string
	node  *aliasfile.Node
}

// Model is the root bubbletea model.
type Model struct {
	cfg *config.Config
	doc *aliasfile.Doc

	screen   screen
	prev     screen
	rows     []row
	cursor   int
	collapse map[string]bool

	filter    textinput.Model
	filtering bool

	alias    aliasForm
	group    groupForm
	settings settingsForm
	confirm  confirmPrompt

	status string
	err    string
	width  int
	height int
	quit   bool
}

// New builds the model. firstRun forces the settings screen.
func New(cfg *config.Config, firstRun bool) (*Model, error) {
	m := &Model{cfg: cfg, collapse: map[string]bool{}, screen: screenList}
	f := textinput.New()
	f.Prompt = "/"
	f.CharLimit = 64
	m.filter = f

	if firstRun {
		m.screen = screenSettings
		m.settings = newSettingsForm(cfg, true)
		m.doc = &aliasfile.Doc{Path: cfg.AliasFile}
		return m, nil
	}
	if err := m.reload(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Model) reload() error {
	doc, err := aliasfile.Load(m.cfg.AliasFile)
	if err != nil {
		return err
	}
	m.doc = doc
	m.rebuild()
	return nil
}

// rebuild flattens the document into the visible row list.
func (m *Model) rebuild() {
	prev := m.currentRow()
	m.rows = nil
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	for _, g := range m.doc.Groups() {
		aliases := m.doc.Aliases(g)
		if q != "" {
			var keep []*aliasfile.Node
			for _, n := range aliases {
				if strings.Contains(strings.ToLower(n.Name), q) ||
					strings.Contains(strings.ToLower(n.Command), q) ||
					strings.Contains(strings.ToLower(g), q) {
					keep = append(keep, n)
				}
			}
			aliases = keep
			if len(aliases) == 0 {
				continue
			}
		}
		m.rows = append(m.rows, row{kind: rowGroup, group: g})
		if m.collapse[g] && q == "" {
			continue
		}
		for _, n := range aliases {
			m.rows = append(m.rows, row{kind: rowAlias, group: g, node: n})
		}
	}
	// Keep the cursor on the same alias across a rebuild where possible.
	if prev != nil && prev.kind == rowAlias {
		for i, r := range m.rows {
			if r.kind == rowAlias && r.node.Name == prev.node.Name {
				m.cursor = i
				return
			}
		}
	}
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) currentRow() *row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *Model) Init() tea.Cmd { return textinput.Blink }

// savedMsg reports the outcome of a save + source cycle.
type savedMsg struct {
	note string
	err  error
}

// save writes the document, validates it and re-sources it.
func (m *Model) save(note string) tea.Cmd {
	doc, cfg := m.doc, m.cfg
	return func() tea.Msg {
		if err := doc.Save(); err != nil {
			return savedMsg{err: err}
		}
		if err := shell.Check(cfg.Shell, cfg.AliasFile); err != nil {
			return savedMsg{err: fmt.Errorf("%w (previous content kept at %s.bak)", err, cfg.AliasFile)}
		}
		src := cfg.SourceFile
		if src == "" {
			src = cfg.AliasFile
		}
		if err := shell.Source(cfg.Shell, src); err != nil {
			return savedMsg{err: err}
		}
		return savedMsg{note: note + " · sourced " + short(src)}
	}
}

func short(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case savedMsg:
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), ""
		} else {
			m.status, m.err = msg.note, ""
		}
		return m, nil
	case tea.KeyMsg:
		m.status, m.err = "", ""
		switch m.screen {
		case screenList:
			return m.updateList(msg)
		case screenAlias:
			return m.updateAliasForm(msg)
		case screenGroup:
			return m.updateGroupForm(msg)
		case screenSettings:
			return m.updateSettings(msg)
		case screenConfirm:
			return m.updateConfirm(msg)
		case screenHelp:
			m.screen = screenList
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		switch msg.String() {
		case "esc":
			m.filtering = false
			m.filter.SetValue("")
			m.filter.Blur()
			m.rebuild()
			return m, nil
		case "enter":
			m.filtering = false
			m.filter.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.rebuild()
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		m.cursor--
		m.clampCursor()
	case "down", "j":
		m.cursor++
		m.clampCursor()
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.rows) - 1
	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, textinput.Blink
	case "esc":
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.rebuild()
		}
	case "tab", "enter":
		if r := m.currentRow(); r != nil && r.kind == rowGroup {
			m.collapse[r.group] = !m.collapse[r.group]
			m.rebuild()
		} else if r != nil {
			return m.openAliasForm(r.node)
		}
	case "a", "n":
		return m.openAliasForm(nil)
	case "e":
		if r := m.currentRow(); r != nil {
			if r.kind == rowAlias {
				return m.openAliasForm(r.node)
			}
			return m.openGroupForm(r.group)
		}
	case "N":
		return m.openGroupForm("")
	case " ":
		if r := m.currentRow(); r != nil && r.kind == rowAlias {
			r.node.Enabled = !r.node.Enabled
			state := "enabled"
			if !r.node.Enabled {
				state = "disabled"
			}
			return m, m.save(fmt.Sprintf("%s %s", r.node.Name, state))
		}
	case "d", "x":
		if r := m.currentRow(); r != nil {
			return m.openConfirm(*r)
		}
	case "s":
		m.prev = screenList
		m.screen = screenSettings
		m.settings = newSettingsForm(m.cfg, false)
		return m, textinput.Blink
	case "r":
		if err := m.reload(); err != nil {
			m.err = err.Error()
		} else {
			m.status = "reloaded " + short(m.cfg.AliasFile)
		}
	case "?", "h":
		m.screen = screenHelp
	}
	return m, nil
}

// groupNames lists groups available for assignment, always including Ungrouped.
func (m *Model) groupNames() []string {
	gs := m.doc.Groups()
	has := false
	for _, g := range gs {
		if g == aliasfile.Ungrouped {
			has = true
		}
	}
	if !has {
		gs = append([]string{aliasfile.Ungrouped}, gs...)
	}
	sortStable(gs)
	return gs
}

// sortStable keeps Ungrouped first, the rest in file order.
func sortStable(gs []string) {
	sort.SliceStable(gs, func(i, j int) bool {
		return gs[i] == aliasfile.Ungrouped && gs[j] != aliasfile.Ungrouped
	})
}
