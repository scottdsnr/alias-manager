# alias-manager

A bubbletea TUI for your shell aliases. View, create, edit, delete, group and
enable/disable aliases in bash or zsh, writing straight to the file you already
keep them in.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/scottdsnr/alias-manager/master/scripts/install.sh | sh
```

This downloads the release binary for your OS/arch into `~/.local/bin`. Set
`BIN_DIR` to install elsewhere, or `VERSION=v0.1.0` to pin a version. Make sure
`~/.local/bin` is on your `PATH`.

From source instead:

```sh
go install github.com/scotthellings/alias-manager@latest
```

## Updating

```sh
alias-manager -update
```

Checks the latest GitHub release, asks before doing anything, then replaces the
binary in place. `alias-manager -version` prints the installed version.

## Releasing

Releases are cut by CI: bump the `VERSION` file on `master` (e.g. `0.2.0`, no
`v` prefix) and push. `.github/workflows/release.yml` runs the tests, builds
linux/darwin amd64+arm64 tarballs, tags `v<VERSION>` and publishes the release
with checksums. A `VERSION` whose tag already exists is skipped.

## First run

On first start you are asked for the path to the file your aliases live in —
`~/.bash_aliases`, `~/.bashrc`, `~/.zshrc`, or an oh-my-zsh custom plugin such
as `~/.oh-my-zsh/custom/aliases.zsh`. Files found on your machine are listed;
`ctrl+n` cycles through them. Settings are reachable later with `s`, or
`alias-manager -setup`, and are stored in `~/.config/alias-manager/config.json`.

Settings also carries the UI accent colour — `←`/`→` cycles teal, green,
magenta, yellow and purple, previewing as you go.

`source` is a second path, used only if the file you edit is not the file that
should be loaded (e.g. you edit an oh-my-zsh plugin but want `.zshrc` sourced).
Leave it blank to use the alias file.

## Keys

| key | action |
|---|---|
| `↑`/`↓`, `k`/`j` | move |
| `enter`/`tab` | fold a group / edit an alias |
| `a` | add alias |
| `N` | new group |
| `e` | edit alias or rename group |
| `c` | duplicate an alias (opens the edit screen pre-filled) |
| `d` | delete (with confirmation) |
| `space` | enable/disable alias |
| `m` | move mode: `space` select (on a group header, the whole group), `a` select all, `enter` pick destination group, `esc` cancel |
| `/` | filter |
| `r` | reload from disk |
| `s` | settings |
| `?` | help |
| `q` | quit |

In the alias form, `←`/`→` moves the alias between groups and `ctrl+e` toggles
enabled/disabled.

## File format

Groups are plain comments, so the file stays a normal shell script:

```sh
# ===== Git =====
alias gs='git status'
#!alias gco='git checkout'
```

A disabled alias keeps the `#!` sentinel so it survives round-trips and can be
re-enabled. Everything else in the file — exports, functions, your own comments
— is preserved untouched. Every save writes a `.bak` beside the file.

## Sourcing

After each change the file is written, syntax-checked with `bash -n`/`zsh -n`,
and sourced in a subshell to prove it loads cleanly. **A child process cannot
modify the shell that launched it**, so to have changes take effect in the
session you are typing in, add the wrapper to your rc file:

```sh
alias-manager -wrapper >> ~/.zshrc   # or ~/.bashrc
```

which defines:

```sh
am() {
  command alias-manager "$@"
  if [ -s '/home/you/.config/alias-manager/unalias.sh' ]; then . '/home/you/.config/alias-manager/unalias.sh'; : > '/home/you/.config/alias-manager/unalias.sh'; fi
  if [ -f '/home/you/.bash_aliases' ]; then . '/home/you/.bash_aliases'; fi
}
```

Then run `am` instead of `alias-manager`. Other open shells pick the changes up
on their next start.

## Trying it safely

`alias-manager` only ever touches the file you point it at, but to try it
without going near your own shell config, run it against a throwaway `$HOME`:

```sh
./scripts/sandbox.sh
```

That builds the binary and launches it with `HOME` and `XDG_CONFIG_HOME` set to
`/tmp/alias-manager-sandbox`, pre-populated with a fake `.bash_aliases`,
`.bashrc`, `.zshrc` and `.oh-my-zsh/custom/aliases.zsh`. The first-run setup
will offer those files; nothing outside the temp dir can be written, and the
config lands in the sandbox rather than `~/.config`.

```sh
./scripts/sandbox.sh        # fresh sandbox each time
./scripts/sandbox.sh -k     # keep the previous sandbox and its edits
./scripts/sandbox.sh -s     # interactive bash inside the sandbox, so you can
                            #   run the aliases and the am() wrapper for real
./scripts/sandbox.sh -- -setup   # pass flags through to alias-manager
```

Inspect what it wrote with `cat /tmp/alias-manager-sandbox/.bash_aliases` and
compare against the `.bak` beside it.

## Development

```sh
go test ./...
```
