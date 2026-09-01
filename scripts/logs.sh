#!/usr/bin/env bash
# Follow both log sources at once: the Lomiri user journal (app start-up, QML
# errors, apparmor denials surfacing through the launcher) and the pulsed log.
# Usage: scripts/logs.sh [journal|pulsed|all] [grep-pattern]
set -uo pipefail

WHAT="${1:-all}"
FILTER="${2:-}"
LOG='$HOME/.cache/pulse/pulsed.log'

ADB=(adb)
[ -n "${ADB_SERIAL:-}" ] && ADB=(adb -s "$ADB_SERIAL")

journal="journalctl --user -n 50 -f"
[ -n "$FILTER" ] && journal="$journal | grep --line-buffered -i '$FILTER'"
journal="$journal | sed -u 's/^/[journal] /'"

pulsed="mkdir -p \$(dirname $LOG); touch $LOG; tail -n 100 -F $LOG | sed -u 's/^/[pulsed]  /'"

case "$WHAT" in
    journal) cmd="$journal" ;;
    pulsed)  cmd="$pulsed" ;;
    all)     cmd="{ $journal & $pulsed ; }" ;;
    *) echo "usage: $0 [journal|pulsed|all] [grep-pattern]" >&2; exit 2 ;;
esac

exec "${ADB[@]}" shell "$cmd"
