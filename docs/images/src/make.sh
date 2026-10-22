#!/bin/bash
# make.sh: rebuild the docs images from a demo fleet (task docs:images).
#
# Terminal pictures come from mrm's real output: status and sync through
# `script` (for colour), the interactive view from its simulated screen
# (internal/tui/screenshot_test.go). Diagrams come from diagrams.py. The web
# UI screenshots are taken by hand: see README-dev.md.
#
# Needs Linux (GNU date, util-linux script), python3 and a built bin/mrm.
set -euo pipefail
cd "$(dirname "$0")/../../.."
ROOT=$PWD
IMG=$ROOT/docs/images
SRC=$IMG/src
TMP=$(mktemp -d)
trap 'rm -r "$TMP"' EXIT

mrm_in() { # <demo dir> <args...>: run mrm against a demo fleet only
  local d=$1; shift
  HOME=$d/home GIT_CONFIG_NOSYSTEM=1 XDG_CONFIG_HOME=$d/xdg/config XDG_CACHE_HOME=$d/xdg/cache \
    XDG_STATE_HOME=$d/xdg/state XDG_RUNTIME_DIR=$d/xdg/run COLUMNS=100 "$ROOT/bin/mrm" "$@"
}
export -f mrm_in
export ROOT

# Sync first, on a fleet whose remotes resolve locally; its history then
# gives the other pictures a "synced 1m ago".
bash "$SRC/demo.sh" "$TMP/sync"
mkdir -p "$TMP/sync/xdg/run"
script -qc "bash -c 'mrm_in $TMP/sync sync'" /dev/null > "$TMP/sync.ansi" || true

bash "$SRC/demo.sh" "$TMP/view" github
mkdir -p "$TMP/view/xdg/run" "$TMP/view/xdg/state/mrm"
cp "$TMP/sync/xdg/state/mrm/runs.jsonl" "$TMP/view/xdg/state/mrm/"
script -qc "bash -c 'mrm_in $TMP/view status'" /dev/null > "$TMP/status.ansi" || true

tui() { # <out.json> <keys> <WxH>
  MRM_SCREENSHOT=$1 MRM_SCREENSHOT_KEYS=$2 MRM_SCREENSHOT_SIZE=$3 \
    MRM_CONFIG=$TMP/view/xdg/config/mrm/config.yaml MRM_SCREENSHOT_CACHE=$TMP/view/xdg/cache/mrm \
    MRM_SCREENSHOT_HOME=$TMP/view/home MRM_SCREENSHOT_HISTORY=$TMP/view/xdg/state/mrm/runs.jsonl \
    go test ./internal/tui -run Screenshot -count=1 > /dev/null
}
tui "$TMP/tui.json" "Gkkk__" 100x17        # two repos marked, cursor below
tui "$TMP/tui-detail.json" "Gl" 120x22     # the detail pane on services/web

python3 "$SRC/term2svg.py" ansi "$TMP/status.ansi" "$IMG/status.svg" --title "mrm status" --prompt "mrm status"
python3 "$SRC/term2svg.py" ansi "$TMP/sync.ansi" "$IMG/sync.svg" --title "mrm sync" --prompt "mrm sync"
python3 "$SRC/term2svg.py" grid "$TMP/tui.json" "$IMG/tui.svg" --title "mrm"
python3 "$SRC/term2svg.py" grid "$TMP/tui-detail.json" "$IMG/tui-detail.svg" --title "mrm  (l: detail pane)"
python3 "$SRC/diagrams.py" "$IMG"
echo "wrote $(ls "$IMG"/*.svg | wc -l) SVGs to docs/images"
