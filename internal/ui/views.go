package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/scotthellings/alias-manager/internal/aliasfile"
	"github.com/scotthellings/alias-manager/internal/config"
	"github.com/scotthellings/alias-manager/internal/shell"
)

// ensureWritable checks the path can be created or written before we adopt it.
func ensureWritable(path string) error {
	if fi, err := os.Stat(path); err == nil {
		if fi.IsDir() {
			return fmt.Errorf("%s is a directory", path)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, fi.Mode().Perm())
		if err != nil {
			return fmt.Errorf("cannot write to %s: %w", path, err)
		}
		return f.Close()
	}
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("directory %s does not exist", dir)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", path, err)
	}
	return f.Close()
}

func (m *Model) View() string {
	if m.quit {
		return ""
	}
	switch m.screen {
	case screenAlias:
		return m.viewAliasForm()
	case screenGroup:
		return m.viewGroupForm()
	case screenSettings:
		return m.viewSettings()
	case screenConfirm:
		return m.viewConfirm()
	case screenHelp:
		return m.viewHelp()
	case screenMove:
		return m.viewMoveForm()
	case screenFunc:
		return m.viewFuncForm()
	default:
		return m.viewList()
	}
}

func (m *Model) footer(keys string) string {
	var b strings.Builder
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(errStyle.Render("✗ "+m.err) + "\n")
	} else if m.status != "" {
		b.WriteString(okStyle.Render("✓ "+m.status) + "\n")
	}
	b.WriteString(helpStyle.Render(keys))
	return b.String()
}

func (m *Model) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" alias manager ") + "  " + helpStyle.Render(short(m.cfg.AliasFile)+" · "+m.cfg.Shell) + "\n\n")
	b.WriteString(m.viewTabs() + "\n\n")

	if m.filtering || m.filter.Value() != "" {
		b.WriteString(m.filter.View() + "\n\n")
	}

	if len(m.rows) == 0 {
		if m.tab == tabFuncs {
			b.WriteString(helpStyle.Render("  no functions yet — press a to add one\n"))
		} else {
			b.WriteString(helpStyle.Render("  no aliases yet — press a to add one, N to make a group\n"))
		}
	}

	// Keep the cursor row visible in a simple sliding window.
	visible := m.height - 10
	if visible < 5 {
		visible = 5
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := start + visible
	if end > len(m.rows) {
		end = len(m.rows)
	}

	for i := start; i < end; i++ {
		r := m.rows[i]
		cur := "  "
		if i == m.cursor {
			cur = cursorStyle.Render("▸ ")
		}
		box := ""
		if m.moving {
			box = "  "
			if r.kind == rowAlias {
				box = helpStyle.Render("☐ ")
				if m.selected[r.node.Name] {
					box = okStyle.Render("☑ ")
				}
			}
		}
		if r.kind == rowGroup {
			marker := "▾"
			if m.collapse[r.group] {
				marker = "▸"
			}
			count := len(m.doc.Aliases(r.group))
			if m.tab == tabFuncs {
				count = len(m.doc.Functions(r.group))
			}
			b.WriteString(fmt.Sprintf("%s%s%s %s %s\n", cur, box, marker,
				groupStyle.Render(r.group), helpStyle.Render(fmt.Sprintf("(%d)", count))))
			continue
		}
		n := r.node
		label, body := n.Name, n.Command
		if r.kind == rowFunc {
			label, body = n.Name+"()", summary(n.Body)
		}
		mark := okStyle.Render("●")
		name := nameStyle.Render(pad(label, 16))
		cmd := cmdStyle.Render(truncate(body, max(20, m.width-34)))
		if !n.Enabled {
			mark = helpStyle.Render("○")
			name = disabledStyle.Render(pad(label, 16))
			cmd = disabledStyle.Render(truncate(body, max(20, m.width-34)))
		}
		b.WriteString(fmt.Sprintf("%s  %s  %s %s %s\n", cur, box, mark, name, cmd))
	}
	if end < len(m.rows) {
		b.WriteString(helpStyle.Render(fmt.Sprintf("    … %d more\n", len(m.rows)-end)))
	}

	if m.moving {
		return b.String() + m.footer(fmt.Sprintf("move mode · %d selected · space select · a select all · enter choose group · esc cancel", len(m.selected)))
	}
	keys := "↑↓ move · 1/2 tab · enter fold/edit · a add · N new group · e edit · c duplicate · space on/off · d delete · / filter · m move · s settings · ? help · q quit"
	if m.tab == tabFuncs {
		keys = "↑↓ move · 1/2 tab · enter fold/edit · a add function · e edit · c duplicate · space on/off · d delete · / filter · s settings · ? help · q quit"
	}
	return b.String() + m.footer(keys)
}

// viewTabs renders the tab bar; 1 and 2 jump straight to a tab.
func (m *Model) viewTabs() string {
	var parts []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d %s (%d) ", i+1, name, m.entryCount(i))
		if i == m.tab {
			parts = append(parts, tabActiveStyle.Render(label))
			continue
		}
		parts = append(parts, tabStyle.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}

// summary flattens a function body to a single line for the list.
func summary(body string) string {
	for _, l := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			rest := ""
			if strings.Count(strings.TrimSpace(body), "\n") > 0 {
				rest = " …"
			}
			return t + rest
		}
	}
	return ""
}

func (m *Model) viewFuncForm() string {
	f := &m.fn
	title := "New function"
	switch {
	case f.oldName != "":
		title = "Edit function: " + f.oldName
	case f.dupOf != "":
		title = "Duplicate of: " + f.dupOf
	}
	group := aliasfile.Ungrouped
	if len(f.groups) > 0 {
		group = f.groups[f.groupIx]
	}
	state := okStyle.Render("enabled")
	if !f.enabled {
		state = errStyle.Render("disabled")
	}

	rows := []string{
		field("name", f.name.View(), f.focus == gName),
		field("body", "", f.focus == gBody),
		f.body.View(),
		field("comment", f.comment.View(), f.focus == gComment),
		field("group", fmt.Sprintf("‹ %s ›", groupStyle.Render(group)), f.focus == gGroup),
		field("state", state+helpStyle.Render("  (ctrl+e toggles)"), false),
	}
	body := boxStyle.Render(strings.Join(rows, "\n"))
	out := titleStyle.Render(" "+title+" ") + "\n\n" + body + "\n"
	if f.err != "" {
		out += "\n" + errStyle.Render("✗ "+f.err) + "\n"
	}
	out += "\n" + helpStyle.Render("writes: ") + cmdStyle.Render(strings.TrimSpace(strings.SplitN(
		(&aliasfile.Node{Name: strings.TrimSpace(f.name.Value()), Comment: strings.TrimSpace(f.comment.Value()), Enabled: f.enabled}).Lines(), "\n", 2)[0])) + "\n"
	return out + m.footer("ctrl+n/ctrl+p field · tab indent (in body) · enter newline (in body) · ctrl+s save · ctrl+e enable/disable · esc cancel")
}

func (m *Model) viewAliasForm() string {
	f := &m.alias
	title := "New alias"
	switch {
	case f.oldName != "":
		title = "Edit alias: " + f.oldName
	case f.dupOf != "":
		title = "Duplicate of: " + f.dupOf
	}
	group := aliasfile.Ungrouped
	if len(f.groups) > 0 {
		group = f.groups[f.groupIx]
	}
	state := okStyle.Render("enabled")
	if !f.enabled {
		state = errStyle.Render("disabled")
	}

	rows := []string{
		field("name", f.inputs[fName].View(), f.focus == fName),
		field("command", f.inputs[fCommand].View(), f.focus == fCommand),
		field("comment", f.inputs[fComment].View(), f.focus == fComment),
		field("group", fmt.Sprintf("‹ %s ›", groupStyle.Render(group)), f.focus == fGroup),
		field("state", state+helpStyle.Render("  (ctrl+e toggles)"), false),
	}
	body := boxStyle.Render(strings.Join(rows, "\n"))
	out := titleStyle.Render(" "+title+" ") + "\n\n" + body + "\n"
	if f.err != "" {
		out += "\n" + errStyle.Render("✗ "+f.err) + "\n"
	}
	preview := (&aliasfile.Node{Name: f.inputs[fName].Value(), Command: f.inputs[fCommand].Value(), Comment: f.inputs[fComment].Value(), Enabled: f.enabled}).Line()
	out += "\n" + helpStyle.Render("writes: ") + cmdStyle.Render(preview) + "\n"
	return out + m.footer("tab next field · ←→ change group · ctrl+e enable/disable · enter save · esc cancel")
}

func (m *Model) viewGroupForm() string {
	title := "New group"
	if m.group.old != "" {
		title = "Rename group: " + m.group.old
	}
	out := titleStyle.Render(" "+title+" ") + "\n\n" + boxStyle.Render(field("name", m.group.input.View(), true)) + "\n"
	if m.group.err != "" {
		out += "\n" + errStyle.Render("✗ "+m.group.err) + "\n"
	}
	out += "\n" + helpStyle.Render("stored in the file as: ") + cmdStyle.Render("# ===== "+m.group.input.Value()+" =====") + "\n"
	return out + m.footer("enter save · esc cancel")
}

func (m *Model) viewConfirm() string {
	c := m.confirm
	var q string
	if c.target.kind == rowFunc {
		q = fmt.Sprintf("Delete function %s?\n%s", nameStyle.Render(c.target.node.Name+"()"), cmdStyle.Render(summary(c.target.node.Body)))
	} else if c.target.kind == rowAlias {
		q = fmt.Sprintf("Delete alias %s?\n%s", nameStyle.Render(c.target.node.Name), cmdStyle.Render(c.target.node.Command))
	} else {
		n := len(m.doc.Aliases(c.target.group)) + len(m.doc.Functions(c.target.group))
		opt := "keep its entries (they move to Ungrouped)"
		if c.withAliases {
			opt = errStyle.Render("delete its " + fmt.Sprint(n) + " entries too")
		}
		q = fmt.Sprintf("Delete group %s?\n%s\n%s", groupStyle.Render(c.target.group), opt, helpStyle.Render("space toggles"))
	}
	return titleStyle.Render(" Confirm ") + "\n\n" + boxStyle.Render(q) + "\n" +
		m.footer("y delete · n cancel")
}

func (m *Model) viewMoveForm() string {
	f := &m.move
	var b strings.Builder
	for i, g := range f.groups {
		cur := "  "
		name := cmdStyle.Render(g)
		if i == f.ix {
			cur = cursorStyle.Render("▸ ")
			name = groupStyle.Render(g)
		}
		b.WriteString(cur + name + "\n")
	}
	out := titleStyle.Render(fmt.Sprintf(" Move %d alias(es) to… ", f.count)) + "\n\n" +
		boxStyle.Render(strings.TrimRight(b.String(), "\n")) + "\n"
	if f.err != "" {
		out += "\n" + errStyle.Render("✗ "+f.err) + "\n"
	}
	return out + m.footer("↑↓ pick group · N new group · enter move · esc back")
}

func (m *Model) viewSettings() string {
	f := &m.settings
	head := " Settings "
	intro := ""
	if f.first {
		head = " Welcome to alias manager "
		intro = helpStyle.Render("First run. Type the path to the file your aliases live in —\n"+
			"e.g. ~/.bash_aliases, ~/.zshrc, or an oh-my-zsh custom plugin\nlike ~/.oh-my-zsh/custom/aliases.zsh.") + "\n\n"
	}
	src := f.inputs[sSourceFile].View()
	rows := []string{
		field("alias file", f.inputs[sAliasFile].View(), f.focus == sAliasFile),
		field("source", src, f.focus == sSourceFile),
		field("shell", fmt.Sprintf("‹ %s ›", groupStyle.Render(f.shell)), f.focus == sShell),
		field("colour", fmt.Sprintf("‹ %s ›", groupStyle.Render(f.color)), f.focus == sColor),
	}
	out := titleStyle.Render(head) + "\n\n" + intro + boxStyle.Render(strings.Join(rows, "\n")) + "\n"

	if len(f.suggest) > 0 {
		var s []string
		for _, c := range f.suggest {
			s = append(s, short(c))
		}
		out += "\n" + helpStyle.Render("found on this machine (ctrl+n cycles): ") + cmdStyle.Render(strings.Join(s, "  ")) + "\n"
	}
	if f.err != "" {
		out += "\n" + errStyle.Render("✗ "+f.err) + "\n"
	}
	return out + m.footer("tab next field · ←→ change shell/colour · ctrl+n suggestion · enter save · esc back")
}

func (m *Model) viewHelp() string {
	src := m.cfg.SourceFile
	if src == "" {
		src = m.cfg.AliasFile
	}
	body := strings.Join([]string{
		groupStyle.Render("Keys"),
		"  ↑/↓ k/j    move            enter      fold group / edit entry",
		"  1 / 2      aliases / functions tab     tab  next tab",
		"  a          add entry       N          new group",
		"  e          edit            c          duplicate entry",
		"  d          delete          /          filter",
		"  space      enable/disable",
		"  m          move mode       (aliases tab · space select · enter pick group)",
		"",
		groupStyle.Render("Function editor"),
		"  ctrl+n / ctrl+p next / previous field    tab  indent by 4 spaces",
		"  enter inserts a newline in the body      ctrl+s  save",
		"  r          reload file     s          settings",
		"  q          quit            ?          this help",
		"",
		groupStyle.Render("How saving works"),
		"  Every change writes the file, runs a shell syntax check, and sources",
		"  the file in a subshell to prove it loads. A child process cannot change",
		"  the shell you launched it from, so to have aliases land in your current",
		"  session, add this to your rc file and use " + nameStyle.Render("am") + ":",
		"",
		cmdStyle.Render(indent(shell.Wrapper("alias-manager", src, config.UnaliasPath()))),
		"",
		groupStyle.Render("File format"),
		"  Groups are comments:   " + cmdStyle.Render("# ===== Git ====="),
		"  Disabled aliases keep: " + cmdStyle.Render("#!alias gs='git status'"),
		"  Functions are whole:   " + cmdStyle.Render("mkcd() { … }") + helpStyle.Render("  (disabled: every line gets #!)"),
		"  A .bak of the previous file is kept next to it on every save.",
	}, "\n")
	return titleStyle.Render(" Help ") + "\n\n" + body + "\n" + m.footer("any key to go back")
}

// ---------- helpers ----------

func field(label, value string, focused bool) string {
	l := labelStyle.Render(label)
	if focused {
		l = labelStyle.Foreground(colAccent).Bold(true).Render(label)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, l, " ", value)
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func truncate(s string, n int) string {
	if n <= 1 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
