#!/usr/bin/env bash
# Regenerates docs/screenshots from the demo workspace.
#
# The data is invented (internal/bbstore/demo.go), so the images contain nobody's
# threads and anyone can reproduce them without a bb server.
#
# Why VHS rather than capturing a tmux pane: a detached tmux mis-reports its
# width to bubbletea, so modals and centred views overflow and render corrupted.
# VHS runs the TUI in a correctly sized headless terminal, so a screenshot shows
# what a reader would actually see. Approach borrowed from TaskYou's
# scripts/qa/ty-qa-shoot.sh.
set -euo pipefail

cd "$(dirname "$0")/.."

command -v vhs    >/dev/null || { echo "vhs not installed (brew install vhs)" >&2; exit 1; }
command -v magick >/dev/null || { echo "imagemagick not installed (brew install imagemagick)" >&2; exit 1; }

W="${SHOT_W:-1600}"
H="${SHOT_H:-660}"
FS="${SHOT_FONTSIZE:-16}"

go build -o bin/bb-tui ./cmd/bb-tui
BIN="$PWD/bin/bb-tui"
mkdir -p docs/screenshots

shot() {
  local name="$1"; shift
  local tape gif frames
  tape="$(mktemp -t bbtui-XXXX).tape"
  gif="$(mktemp -t bbtui-XXXX).gif"

  {
    echo "Output \"$gif\""
    echo "Set FontSize $FS"
    echo "Set Width $W"
    echo "Set Height $H"
    echo "Set Padding 24"
    echo 'Set Shell "bash"'
    # VHS starts without a UTF-8 locale, and the TUI falls back to ASCII glyphs
    # when it cannot see one — the screenshots would show "o" and "!" where a
    # reader sees ◦ and ⚠.
    echo 'Env LANG "en_US.UTF-8"'
    echo 'Env LC_ALL "en_US.UTF-8"'
    echo 'Hide'
    echo 'Type "clear"'
    echo 'Enter'
    echo 'Show'
    echo "Type \"$BIN --demo\""
    echo 'Enter'
    echo 'Sleep 3s'
    for line in "$@"; do echo "$line"; done
  } > "$tape"

  vhs "$tape" >/dev/null

  # VHS's own Screenshot command is unreliable across versions; the dependable
  # path is to coalesce the recorded gif and keep its final frame.
  frames="$(mktemp -d)"
  magick "$gif" -coalesce "$frames/f_%04d.png"
  cp "$(ls "$frames"/f_*.png | tail -1)" "docs/screenshots/$name.png"
  rm -rf "$frames" "$tape" "$gif"
  echo "  docs/screenshots/$name.png"
}

echo "writing screenshots:"
# "P" focuses the Running column, whose first card is the thread that has a
# conversation seeded against it.
shot board
shot thread    'Type "P"' 'Sleep 500ms' 'Enter' 'Sleep 2s'
shot composer  'Type "P"' 'Sleep 500ms' 'Enter' 'Sleep 2s' 'Type "i"' 'Sleep 500ms' 'Type "Drain the old endpoint once the consumer has caught up."' 'Sleep 1s'
shot skills    'Type "P"' 'Sleep 500ms' 'Enter' 'Sleep 2s' 'Type "i"' 'Sleep 500ms' 'Type "/de"' 'Sleep 1500ms'
shot list      'Type "v"' 'Sleep 2s'
