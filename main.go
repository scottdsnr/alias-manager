// Command alias-manager is a terminal UI for viewing, creating, editing,
// grouping and disabling shell aliases in bash and zsh.
package main

import (
	"bufio"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/scotthellings/alias-manager/internal/config"
	"github.com/scotthellings/alias-manager/internal/shell"
	"github.com/scotthellings/alias-manager/internal/ui"
	"github.com/scotthellings/alias-manager/internal/update"
)

// versionFile is the VERSION file baked in at build time; release builds
// override version via -ldflags "-X main.version=...".
//
//go:embed VERSION
var versionFile string

var version string

func init() {
	if version == "" {
		version = strings.TrimSpace(versionFile)
	}
}

func main() {
	setup := flag.Bool("setup", false, "open the settings screen on start")
	printWrapper := flag.Bool("wrapper", false, "print the shell function that reloads aliases in your current shell")
	doUpdate := flag.Bool("update", false, "check for a newer release and install it")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("alias-manager", version)
		return
	}

	if *doUpdate {
		if err := runUpdate(); err != nil {
			fail(err)
		}
		return
	}

	cfg, configured, err := config.Load()
	if err != nil {
		fail(err)
	}

	if *printWrapper {
		src := cfg.SourceFile
		if src == "" {
			src = cfg.AliasFile
		}
		if src == "" {
			fail(fmt.Errorf("not configured yet — run alias-manager first"))
		}
		fmt.Println(shell.Wrapper("alias-manager", src))
		return
	}

	m, err := ui.New(cfg, !configured || *setup)
	if err != nil {
		fail(err)
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fail(err)
	}
}

// runUpdate compares the running version against the latest release and,
// with the user's consent, replaces the binary.
func runUpdate() error {
	fmt.Println("current version:", version)
	fmt.Println("checking for updates...")

	rel, err := update.Latest()
	if err != nil {
		return err
	}
	if !update.Newer(version, rel.TagName) {
		fmt.Println("already up to date.")
		return nil
	}

	fmt.Printf("%s is available (%s)\n", rel.TagName, rel.HTMLURL)
	fmt.Print("update now? [y/N] ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
		fmt.Println("cancelled.")
		return nil
	}

	fmt.Println("downloading", update.AssetName())
	if err := update.Apply(rel); err != nil {
		return err
	}
	fmt.Println("updated to", rel.TagName)
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "alias-manager:", err)
	os.Exit(1)
}
