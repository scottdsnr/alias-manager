package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/scotthellings/alias-manager/internal/config"
)

// isolate points HOME and the config dir at a temp dir, so no test can read or
// write the config of the person running them.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	return dir
}

func newTestModel(t *testing.T, content string) *Model {
	t.Helper()
	dir := isolate(t)
	p := filepath.Join(dir, "aliases")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{AliasFile: p, SourceFile: p, Shell: "bash"}
	m, err := New(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 30
	return m
}

func key(m *Model, s string) *Model {
	var msg tea.Msg
	switch s {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "space":
		msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	next, cmd := m.Update(msg)
	if !saveKeys[s] {
		// Typing returns a cursor-blink command that never completes off-screen.
		return next.(*Model)
	}
	return drain(next.(*Model), cmd)
}

// saveKeys are the keys that trigger a write, and so the only ones whose
// command is worth running in tests.
var saveKeys = map[string]bool{"enter": true, "space": true, "y": true}

// drain runs a returned command the way the bubbletea runtime would, so that
// asynchronous work (saving and sourcing the file) lands before we assert.
// A command that produces nothing is abandoned after a short wait.
func drain(m *Model, cmd tea.Cmd) *Model {
	if cmd == nil {
		return m
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if msg == nil {
			return m
		}
		next, _ := m.Update(msg)
		return next.(*Model)
	case <-time.After(1500 * time.Millisecond):
		return m
	}
}

func typ(m *Model, s string) *Model {
	for _, r := range s {
		m = key(m, string(r))
	}
	return m
}

const fixture = "# ===== Git =====\nalias gs='git status'\nalias gco='git checkout'\n"

func TestListShowsGroupsAndAliases(t *testing.T) {
	m := newTestModel(t, fixture)
	v := m.View()
	for _, want := range []string{"Git", "gs", "git status", "gco"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view missing %q:\n%s", want, v)
		}
	}
}

func TestCollapseGroupHidesAliases(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "enter") // cursor starts on the group header
	if strings.Contains(m.View(), "git status") {
		t.Fatalf("collapsed group still lists aliases:\n%s", m.View())
	}
}

func TestFilterNarrowsRows(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "/")
	m = typ(m, "gco")
	v := m.View()
	if strings.Contains(v, "git status") || !strings.Contains(v, "gco") {
		t.Fatalf("filter wrong:\n%s", v)
	}
}

func TestCreateAliasWritesFile(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "a")
	m = typ(m, "gp")
	m = key(m, "tab")
	m = typ(m, "git push")
	m = key(m, "enter")
	if m.screen != screenList {
		t.Fatalf("form still open: %s", m.alias.err)
	}
	if n := m.doc.Find("gp"); n == nil || n.Command != "git push" {
		t.Fatalf("alias not added: %+v", n)
	}
	b, _ := os.ReadFile(m.cfg.AliasFile)
	if !strings.Contains(string(b), "alias gp='git push'") {
		t.Fatalf("file:\n%s", b)
	}
}

func TestDuplicateNameShowsErrorAndKeepsForm(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "a")
	m = typ(m, "gs")
	m = key(m, "tab")
	m = typ(m, "x")
	m = key(m, "enter")
	if m.screen != screenAlias || m.alias.err == "" {
		t.Fatalf("expected a duplicate error, screen=%v err=%q", m.screen, m.alias.err)
	}
}

func TestToggleDisableWritesSentinel(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "down") // onto gs
	m = key(m, "space")
	if m.doc.Find("gs").Enabled {
		t.Fatal("gs should be disabled")
	}
	b, _ := os.ReadFile(m.cfg.AliasFile)
	if !strings.Contains(string(b), "#!alias gs=") {
		t.Fatalf("file:\n%s", b)
	}
}

func TestDeleteAliasNeedsConfirmation(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "down")
	m = key(m, "d")
	if m.screen != screenConfirm {
		t.Fatal("expected confirm screen")
	}
	m = key(m, "n")
	if m.doc.Find("gs") == nil {
		t.Fatal("cancel should not delete")
	}
	m = key(m, "d")
	m = key(m, "y")
	if m.doc.Find("gs") != nil {
		t.Fatal("gs should be deleted")
	}
}

func TestCreateGroupAndAssignAlias(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "N")
	m = typ(m, "Docker")
	m = key(m, "enter")
	if got := strings.Join(m.doc.Groups(), ","); got != "Git,Docker" {
		t.Fatalf("groups = %s", got)
	}
	// Move gs into the new group via the edit form's group selector.
	m.cursor = 1
	m = key(m, "e")
	m.alias.focus = fGroup
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = drain(next.(*Model), cmd)
	m = key(m, "enter")
	if n := m.doc.Find("gs"); n == nil || n.Group != "Docker" {
		t.Fatalf("gs group = %+v", n)
	}
}

func TestFirstRunOpensSettings(t *testing.T) {
	dir := isolate(t)
	cfg := &config.Config{Shell: "bash"}
	m, err := New(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if m.screen != screenSettings {
		t.Fatal("expected settings screen on first run")
	}
	if !strings.Contains(m.View(), "First run") {
		t.Fatalf("view:\n%s", m.View())
	}
	m = typ(m, filepath.Join(dir, "aliases"))
	m = key(m, "enter")
	if m.screen != screenList || m.settings.err != "" {
		t.Fatalf("setup failed: %q", m.settings.err)
	}
	if _, err := os.Stat(config.Path()); err != nil {
		t.Fatalf("config not written to the isolated dir: %v", err)
	}
	if !strings.HasPrefix(config.Path(), dir) {
		t.Fatalf("config escaped the temp dir: %s", config.Path())
	}
}

func TestSettingsRejectsUnwritablePath(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "s")
	m.settings.inputs[sAliasFile].SetValue("/nope/definitely/missing/aliases")
	m = key(m, "enter")
	if m.settings.err == "" || m.screen != screenSettings {
		t.Fatal("expected a path error")
	}
}
