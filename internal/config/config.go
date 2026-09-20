package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config is persisted to ~/.config/alias-manager/config.json.
type Config struct {
	// AliasFile is the file aliases are read from and written to.
	AliasFile string `json:"alias_file"`
	// SourceFile is the file sourced after a save. Usually the alias file
	// itself, but for an oh-my-zsh plugin it may be .zshrc instead.
	SourceFile string `json:"source_file"`
	// Shell is "bash" or "zsh"; used to validate and source.
	Shell string `json:"shell"`
	// Color names the UI accent colour. Empty means the UI default.
	Color string `json:"color"`

	path string
}

func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "alias-manager")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "alias-manager")
}

func Path() string { return filepath.Join(Dir(), "config.json") }

// UnaliasPath is the scratch file of `unalias` lines the shell wrapper runs
// before re-sourcing, so removed or renamed aliases leave the live shell.
func UnaliasPath() string { return filepath.Join(Dir(), "unalias.sh") }

// Load reads the config. It returns a zero-valued Config with ok=false when no
// config exists yet, which is the signal to run first-time setup.
func Load() (*Config, bool, error) {
	p := Path()
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{path: p, Shell: DetectShell()}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	c := &Config{path: p}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, false, err
	}
	if c.Shell == "" {
		c.Shell = DetectShell()
	}
	if c.SourceFile == "" {
		c.SourceFile = c.AliasFile
	}
	return c, c.AliasFile != "", nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(b, '\n'), 0o644)
}

// DetectShell guesses the user's login shell.
func DetectShell() string {
	s := os.Getenv("SHELL")
	if strings.Contains(s, "zsh") {
		return "zsh"
	}
	if strings.Contains(s, "bash") {
		return "bash"
	}
	if os.Getenv("ZSH_VERSION") != "" {
		return "zsh"
	}
	return "bash"
}

// ExpandPath resolves ~ and environment variables, and makes the path absolute.
func ExpandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// Candidates lists the conventional alias files that exist on this machine,
// offered as suggestions during setup.
func Candidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	rel := []string{
		".bash_aliases",
		".bashrc",
		".zshrc",
		".aliases",
		".oh-my-zsh/custom/aliases.zsh",
	}
	// Any other custom oh-my-zsh plugin/custom file is a valid target too.
	if m, err := filepath.Glob(filepath.Join(home, ".oh-my-zsh/custom/*.zsh")); err == nil {
		for _, p := range m {
			rel = append(rel, strings.TrimPrefix(p, home+string(filepath.Separator)))
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range rel {
		p := filepath.Join(home, r)
		if seen[p] {
			continue
		}
		seen[p] = true
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}
