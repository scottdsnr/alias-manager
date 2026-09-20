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
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		msg = tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+n":
		msg = tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+e":
		msg = tea.KeyMsg{Type: tea.KeyCtrlE}
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
var saveKeys = map[string]bool{"enter": true, "space": true, "y": true, "ctrl+s": true}

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

const moveFixture = "alias ll='ls -l'\nalias la='ls -a'\n\n# ===== Git =====\nalias gs='git status'\n"

func TestMoveModeMovesSelectedAliases(t *testing.T) {
	m := newTestModel(t, moveFixture)
	m = key(m, "m") // cursor is on the Ungrouped header
	m = key(m, "down")
	m = key(m, "space") // select ll, cursor advances
	m = key(m, "space") // select la
	if len(m.selected) != 2 {
		t.Fatalf("want 2 selected, got %v", m.selected)
	}
	m = key(m, "enter") // group picker
	if m.screen != screenMove {
		t.Fatalf("want move picker, got screen %v", m.screen)
	}
	if m.move.groups[m.move.ix] != "Git" {
		t.Fatalf("picker should start on the cursor's group, got %q", m.move.groups[m.move.ix])
	}
	m = key(m, "enter")
	if m.moving || len(m.selected) != 0 {
		t.Fatal("move mode should end after a move")
	}
	for _, n := range []string{"ll", "la"} {
		if g := m.doc.Find(n).Group; g != "Git" {
			t.Fatalf("%s in group %q, want Git", n, g)
		}
	}
	out, err := os.ReadFile(m.cfg.AliasFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "# ===== Git =====\nalias gs='git status'\nalias ll='ls -l'\nalias la='ls -a'") {
		t.Fatalf("unexpected file:\n%s", out)
	}
}

func TestMoveModeEscCancels(t *testing.T) {
	m := newTestModel(t, moveFixture)
	m = key(m, "m")
	m = key(m, "down")
	m = key(m, "space")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if m.moving || len(m.selected) != 0 {
		t.Fatal("esc should leave move mode with nothing selected")
	}
	if g := m.doc.Find("ll").Group; g != "Ungrouped" {
		t.Fatalf("alias moved despite cancel: %q", g)
	}
}

func TestMoveModeGroupHeaderSelectsWholeGroup(t *testing.T) {
	m := newTestModel(t, moveFixture)
	m = key(m, "m")
	m = key(m, "space") // on the Ungrouped header
	if len(m.selected) != 2 {
		t.Fatalf("want the whole group selected, got %v", m.selected)
	}
	m = key(m, "space") // toggles the group back off
	if len(m.selected) != 0 {
		t.Fatalf("want selection cleared, got %v", m.selected)
	}
}

func TestDuplicateAliasKeepsOriginal(t *testing.T) {
	m := newTestModel(t, fixture)
	m = key(m, "down") // onto gs
	m = key(m, "c")
	if m.screen != screenAlias || m.alias.dupOf != "gs" {
		t.Fatalf("duplicate form not open: %+v", m.alias)
	}
	if got := m.alias.inputs[fName].Value(); got != "gs-copy" {
		t.Fatalf("suggested name = %q", got)
	}
	m = key(m, "enter")
	orig, dup := m.doc.Find("gs"), m.doc.Find("gs-copy")
	if orig == nil || dup == nil || dup.Command != orig.Command || dup.Group != orig.Group {
		t.Fatalf("orig=%+v dup=%+v", orig, dup)
	}
	b, _ := os.ReadFile(m.cfg.AliasFile)
	if !strings.Contains(string(b), "alias gs-copy='git status'") {
		t.Fatalf("file:\n%s", b)
	}
}

func TestDuplicateNameAvoidsCollisions(t *testing.T) {
	m := newTestModel(t, fixture+"alias gs-copy='git status'\n")
	if got := m.copyName("gs"); got != "gs-copy-2" {
		t.Fatalf("copyName = %q", got)
	}
}

func TestSettingsCyclesAndSavesColor(t *testing.T) {
	m := newTestModel(t, "alias ll='ls -l'\n")
	m.screen = screenSettings
	m.settings = newSettingsForm(m.cfg, false)
	if m.settings.color != DefaultColor {
		t.Fatalf("want default colour, got %q", m.settings.color)
	}
	m.settings.focus = sColor
	key(m, "right")
	if m.settings.color != Colors[1] {
		t.Fatalf("want %q, got %q", Colors[1], m.settings.color)
	}
	key(m, "left")
	key(m, "left")
	want := Colors[len(Colors)-1]
	if m.settings.color != want {
		t.Fatalf("want wrap to %q, got %q", want, m.settings.color)
	}
	if _, _ = m.submitSettings(); m.settings.err != "" {
		t.Fatalf("save failed: %s", m.settings.err)
	}
	if m.cfg.Color != want {
		t.Fatalf("config colour %q, want %q", m.cfg.Color, want)
	}
	if colAccent != accents[want] {
		t.Fatal("accent style not applied")
	}
	applyColor(DefaultColor)
}

func TestSettingsEscRevertsColorPreview(t *testing.T) {
	m := newTestModel(t, "alias ll='ls -l'\n")
	m.cfg.Color = "green"
	applyColor(m.cfg.Color)
	m.screen = screenSettings
	m.settings = newSettingsForm(m.cfg, false)
	m.settings.focus = sColor
	key(m, "right")
	key(m, "esc")
	if colAccent != accents["green"] {
		t.Fatal("esc should revert the preview to the saved colour")
	}
	applyColor(DefaultColor)
}

const funcFixture = "# ===== Git =====\nalias gs='git status'\n\ngsync() { # pull, then push\n    git pull --rebase\n    git push\n}\n"

func TestTabKeysSwitchBetweenAliasesAndFunctions(t *testing.T) {
	m := newTestModel(t, funcFixture)
	if !strings.Contains(m.View(), "git status") {
		t.Fatal("aliases tab should list aliases")
	}
	m = key(m, "2")
	if m.tab != tabFuncs {
		t.Fatalf("tab = %d", m.tab)
	}
	v := m.View()
	if !strings.Contains(v, "gsync()") || strings.Contains(v, "git status") {
		t.Fatalf("functions tab wrong:\n%s", v)
	}
	m = key(m, "1")
	if m.tab != tabAliases || !strings.Contains(m.View(), "git status") {
		t.Fatalf("did not return to aliases:\n%s", m.View())
	}
}

func TestCreateFunctionWritesFile(t *testing.T) {
	m := newTestModel(t, funcFixture)
	m = key(m, "2")
	m = key(m, "a")
	m = typ(m, "mkcd")
	m = key(m, "ctrl+n") // into the body
	m = typ(m, "mkdir -p x")
	m = key(m, "ctrl+s")
	if m.screen != screenList {
		t.Fatalf("form still open: %s", m.fn.err)
	}
	n := m.doc.FindFunc("mkcd")
	if n == nil || !strings.Contains(n.Body, "mkdir -p x") {
		t.Fatalf("function not added: %+v", n)
	}
	b, _ := os.ReadFile(m.cfg.AliasFile)
	if !strings.Contains(string(b), "mkcd() {\n    mkdir -p x\n}") {
		t.Fatalf("file:\n%s", b)
	}
}

func TestFunctionBodyTakesNewlinesAndTabs(t *testing.T) {
	m := newTestModel(t, funcFixture)
	m = key(m, "2")
	m = key(m, "a")
	m = typ(m, "two")
	m = key(m, "ctrl+n")
	m = typ(m, "echo one")
	m = key(m, "enter") // newline inside the body, not a save
	if m.screen != screenFunc {
		t.Fatal("enter in the body should not submit the form")
	}
	m = key(m, "tab") // indent by four spaces
	m = typ(m, "echo two")
	m = key(m, "ctrl+s")
	n := m.doc.FindFunc("two")
	if n == nil {
		t.Fatalf("not created: %s", m.fn.err)
	}
	if n.Body != "    echo one\n    echo two" {
		t.Fatalf("body = %q", n.Body)
	}
}

func TestToggleAndDeleteFunction(t *testing.T) {
	m := newTestModel(t, funcFixture)
	m = key(m, "2")
	m = key(m, "down") // onto gsync
	m = key(m, "space")
	if m.doc.FindFunc("gsync").Enabled {
		t.Fatal("gsync should be disabled")
	}
	b, _ := os.ReadFile(m.cfg.AliasFile)
	if !strings.Contains(string(b), "#!gsync() {") || !strings.Contains(string(b), "#!    git push") {
		t.Fatalf("file:\n%s", b)
	}
	// Disabling must queue an unset -f so the live shell drops it too.
	if !m.staleFn["gsync"] {
		t.Fatal("expected gsync to be marked stale")
	}
	m = key(m, "d")
	m = key(m, "y")
	if m.doc.FindFunc("gsync") != nil {
		t.Fatal("gsync should be deleted")
	}
}

func TestEditFunctionKeepsCommentAndGroup(t *testing.T) {
	m := newTestModel(t, funcFixture)
	m = key(m, "2")
	m = key(m, "down")
	m = key(m, "e")
	if m.screen != screenFunc || m.fn.oldName != "gsync" {
		t.Fatalf("edit did not open: screen=%v", m.screen)
	}
	m = key(m, "ctrl+n")
	m = typ(m, "echo done")
	m = key(m, "ctrl+s")
	n := m.doc.FindFunc("gsync")
	if n == nil || n.Comment != "pull, then push" || n.Group != "Git" {
		t.Fatalf("gsync = %+v", n)
	}
	if !strings.Contains(n.Body, "echo done") {
		t.Fatalf("body = %q", n.Body)
	}
}
