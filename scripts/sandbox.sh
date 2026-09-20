#!/usr/bin/env bash
# Run alias-manager against a throwaway HOME so it can never touch your real
# shell config. Everything lives under a temp dir that is printed on exit.
#
#   ./scripts/sandbox.sh          # fresh sandbox, first-run setup
#   ./scripts/sandbox.sh -k       # keep and reuse the previous sandbox
#   ./scripts/sandbox.sh -s       # drop into a shell inside the sandbox
#   ./scripts/sandbox.sh -- -setup  # pass flags through to alias-manager
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
box="${TMPDIR:-/tmp}/alias-manager-sandbox"
keep=0
spawn_shell=0
while getopts "ks" opt; do
  case "$opt" in
    k) keep=1 ;;
    s) spawn_shell=1 ;;
    *) exit 2 ;;
  esac
done
shift $((OPTIND - 1))

if [ "$keep" -eq 0 ]; then
  rm -rf "$box"
fi

if [ ! -d "$box" ]; then
  mkdir -p "$box/.oh-my-zsh/custom" "$box/.config"

  cat > "$box/.bash_aliases" <<'ALIASES'
# ===== Git =====
alias gs='git status'
alias gco='git checkout'
#!alias gp='git push --force'

# ===== Files =====
alias ll='ls -la'
alias ..='cd ..'
ALIASES

  cat > "$box/.bashrc" <<'BASHRC'
# sandbox .bashrc
export EDITOR=vim
[ -f ~/.bash_aliases ] && . ~/.bash_aliases

# The wrapper under test: re-sources the alias file in *this* shell on exit.
am() {
  command alias-manager "$@"
  [ -f ~/.bash_aliases ] && . ~/.bash_aliases
}
PS1='[sandbox] \w $ '
BASHRC

  cat > "$box/.zshrc" <<'ZSHRC'
# sandbox .zshrc
export EDITOR=vim
alias zz='echo zsh alias'
ZSHRC

  cat > "$box/.oh-my-zsh/custom/aliases.zsh" <<'OMZ'
# ===== Kubernetes =====
alias k='kubectl'
alias kgp='kubectl get pods'
OMZ

  echo "created sandbox home: $box"
fi

go build -o "$box/alias-manager" "$repo"

echo "sandbox HOME=$box"
echo "alias files: .bash_aliases  .bashrc  .zshrc  .oh-my-zsh/custom/aliases.zsh"
echo

if [ "$spawn_shell" -eq 1 ]; then
  # An interactive shell whose HOME, config and rc files are all inside the box.
  exec env -i \
    HOME="$box" XDG_CONFIG_HOME="$box/.config" \
    TERM="${TERM:-xterm-256color}" PATH="$box:$PATH" SHELL=/bin/bash \
    /bin/bash --rcfile "$box/.bashrc" -i
fi

exec env HOME="$box" XDG_CONFIG_HOME="$box/.config" SHELL=/bin/bash \
  "$box/alias-manager" "$@"
