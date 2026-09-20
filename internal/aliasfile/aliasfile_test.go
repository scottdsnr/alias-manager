package aliasfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `# my shell config
export EDITOR=vim

alias ll='ls -la'

# ===== Git =====
alias gs='git status'
#!alias gco="git checkout" # disabled on purpose

# ===== Docker =====
alias dps='docker ps'
`

func load(t *testing.T, content string) *Doc {
	t.Helper()
	p := filepath.Join(t.TempDir(), "aliases")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParseGroupsAndState(t *testing.T) {
	d := load(t, sample)
	if got := d.Groups(); strings.Join(got, ",") != "Ungrouped,Git,Docker" {
		t.Fatalf("groups = %v", got)
	}
	if n := d.Find("ll"); n == nil || n.Group != Ungrouped || !n.Enabled {
		t.Fatalf("ll = %+v", n)
	}
	n := d.Find("gco")
	if n == nil || n.Enabled || n.Group != "Git" || n.Command != "git checkout" || n.Comment != "disabled on purpose" {
		t.Fatalf("gco = %+v", n)
	}
}

func TestRoundTripPreservesUnrelatedLines(t *testing.T) {
	d := load(t, sample)
	if got := d.Render(); got != sample {
		t.Fatalf("round trip changed file:\n--got--\n%s\n--want--\n%s", got, sample)
	}
}

func TestUpsertPlacesAliasInItsGroup(t *testing.T) {
	d := load(t, sample)
	if err := d.Upsert("", Node{Name: "gp", Command: "git push", Enabled: true, Group: "Git"}); err != nil {
		t.Fatal(err)
	}
	out := d.Render()
	git := strings.Index(out, "# ===== Git =====")
	docker := strings.Index(out, "# ===== Docker =====")
	gp := strings.Index(out, "alias gp=")
	if !(git < gp && gp < docker) {
		t.Fatalf("gp landed outside the Git section:\n%s", out)
	}
}

func TestUpsertMovesGroupAndRejectsDuplicates(t *testing.T) {
	d := load(t, sample)
	if err := d.Upsert("ll", Node{Name: "ll", Command: "ls -la", Enabled: true, Group: "Docker"}); err != nil {
		t.Fatal(err)
	}
	if n := d.Find("ll"); n.Group != "Docker" {
		t.Fatalf("group = %q", n.Group)
	}
	if err := d.Upsert("", Node{Name: "gs", Command: "x", Group: "Git"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestToggleDisableRendersSentinel(t *testing.T) {
	d := load(t, sample)
	d.Find("gs").Enabled = false
	if !strings.Contains(d.Render(), "#!alias gs='git status'") {
		t.Fatalf("missing sentinel:\n%s", d.Render())
	}
}

func TestDeleteGroupKeepsOrDropsAliases(t *testing.T) {
	d := load(t, sample)
	d.DeleteGroup("Git", false)
	if n := d.Find("gs"); n == nil || n.Group != Ungrouped {
		t.Fatalf("gs = %+v", n)
	}
	d.DeleteGroup("Docker", true)
	if d.Find("dps") != nil {
		t.Fatal("dps should be gone")
	}
}

func TestQuoteSelectionForCommandsContainingQuotes(t *testing.T) {
	d := &Doc{}
	if err := d.Upsert("", Node{Name: "say", Command: `echo 'hi'`, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(d.Render())
	if got != `alias say="echo 'hi'"` {
		t.Fatalf("got %s", got)
	}
}

func TestSaveKeepsBackupAndMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "aliases")
	if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	d, _ := Load(p)
	d.Delete("gs")
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(p + ".bak")
	if err != nil || string(bak) != sample {
		t.Fatalf("backup wrong: %v", err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

func TestValidateName(t *testing.T) {
	for _, bad := range []string{"", "  ", "a b", "a=b", "a$b"} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("%q should be invalid", bad)
		}
	}
	for _, ok := range []string{"gs", "g.s", "g-s", "..", "l1"} {
		if err := ValidateName(ok); err != nil {
			t.Fatalf("%q should be valid: %v", ok, err)
		}
	}
}

const funcSample = `alias ll='ls -la'

# ===== Git =====
gsync() { # pull, then push
    git pull --rebase
    git push
}

#!stale() {
#!    echo gone
#!}

if [ -n "$X" ]; then
    echo not a function
fi
`

func TestParseFunctionsAndRoundTrip(t *testing.T) {
	d := load(t, funcSample)
	if got := d.Render(); got != funcSample {
		t.Fatalf("round trip changed file:\n--got--\n%s\n--want--\n%s", got, funcSample)
	}
	n := d.FindFunc("gsync")
	if n == nil || !n.Enabled || n.Group != "Git" || n.Comment != "pull, then push" {
		t.Fatalf("gsync = %+v", n)
	}
	if n.Body != "    git pull --rebase\n    git push" {
		t.Fatalf("body = %q", n.Body)
	}
	if s := d.FindFunc("stale"); s == nil || s.Enabled {
		t.Fatalf("stale = %+v", s)
	}
	if d.FindFunc("then") != nil || len(d.Functions("Git")) != 2 {
		t.Fatalf("an if block was read as a function: %d", len(d.Functions("Git")))
	}
}

func TestDisablingAFunctionCommentsEveryLine(t *testing.T) {
	d := load(t, funcSample)
	d.FindFunc("gsync").Enabled = false
	out := d.Render()
	for _, want := range []string{"#!gsync() { # pull, then push", "#!    git pull --rebase", "#!}"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// And it must parse back to exactly the same function.
	d2 := load(t, out)
	n := d2.FindFunc("gsync")
	if n == nil || n.Enabled || n.Body != "    git pull --rebase\n    git push" {
		t.Fatalf("re-parsed = %+v", n)
	}
}

func TestUpsertFuncValidatesAndPlacesInGroup(t *testing.T) {
	d := load(t, funcSample)
	if err := d.UpsertFunc("", Node{Name: "mkcd", Enabled: true, Group: "Git"}); err == nil {
		t.Fatal("expected an empty-body error")
	}
	if err := d.UpsertFunc("", Node{Name: "2bad", Body: "echo x", Enabled: true}); err == nil {
		t.Fatal("expected an invalid-name error")
	}
	if err := d.UpsertFunc("", Node{Name: "mkcd", Body: "    mkdir -p \"$1\"", Enabled: true, Group: "Git"}); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertFunc("", Node{Name: "mkcd", Body: "x", Enabled: true, Group: "Git"}); err == nil {
		t.Fatal("expected a duplicate-name error")
	}
	if n := d.FindFunc("mkcd"); n == nil || n.Group != "Git" {
		t.Fatalf("mkcd = %+v", n)
	}
	d.DeleteFunc("mkcd")
	if d.FindFunc("mkcd") != nil {
		t.Fatal("mkcd should be gone")
	}
}

// An alias and a function may share a name without colliding.
func TestAliasAndFunctionNamespacesAreSeparate(t *testing.T) {
	d := load(t, funcSample)
	if err := d.UpsertFunc("", Node{Name: "ll", Body: "    ls -la", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if d.Find("ll") == nil || d.FindFunc("ll") == nil {
		t.Fatal("both an alias and a function named ll should exist")
	}
	d.DeleteFunc("ll")
	if d.Find("ll") == nil {
		t.Fatal("deleting the function removed the alias")
	}
}
