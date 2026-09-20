package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "aliases")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckAcceptsValidAndRejectsBroken(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		if _, err := resolve(sh); err != nil {
			t.Logf("skipping %s: %v", sh, err)
			continue
		}
		if err := Check(sh, write(t, "alias gs='git status'\n")); err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		if err := Check(sh, write(t, "alias gs='unterminated\nif then\n")); err == nil {
			t.Fatalf("%s: expected a syntax error", sh)
		}
	}
}

func TestSourceRunsTheFile(t *testing.T) {
	sh := "bash"
	if _, err := resolve(sh); err != nil {
		t.Skip(err)
	}
	if err := Source(sh, write(t, "alias gs='git status'\n")); err != nil {
		t.Fatal(err)
	}
	if err := Source(sh, write(t, "exit 3\n")); err == nil {
		t.Fatal("expected failure")
	}
}

func TestWrapperQuotesPath(t *testing.T) {
	w := Wrapper("alias-manager", "/home/a b/.bash_aliases")
	if !strings.Contains(w, `'/home/a b/.bash_aliases'`) || !strings.Contains(w, "command alias-manager") {
		t.Fatalf("got:\n%s", w)
	}
}
