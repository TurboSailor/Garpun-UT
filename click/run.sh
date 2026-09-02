#!/bin/bash
# Click entry point: bring up the pulsed daemon (once), then hand over to the UI.
#
# The daemon runs as its own transient systemd --user unit, NOT as a child of
# this script. Lomiri suspends a backgrounded app by sending SIGSTOP to the
# app-launch unit's cgroup and later stops the unit outright (KillMode=
# control-group). A plain `setsid` child leaves the session but stays in that
# cgroup, so the daemon froze mid-sync and was killed together with the UI.
# A separate unit gets a separate cgroup, so background sync and notifications
# survive the UI being suspended, killed or restarted. The UI only talks to it
# over http://127.0.0.1:21830.
set -u

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/pulse"
LOG="$CACHE_DIR/pulsed.log"
WD_LOG="$CACHE_DIR/pulse-wdnotify.log"
DAEMON_UNIT=pulse-pulsed
WD_UNIT=pulse-wdnotify
MAX_LOG=5242880

# /proc/net/tcp keeps ports in uppercase hex: 21830 == 0x5546, state 0A == LISTEN.
PORT_HEX=5546

mkdir -p "$CACHE_DIR" "$RUNTIME_DIR" 2>/dev/null

# A stuck BLE retry loop can grow the log without bound. Keep the tail aside and
# truncate in place: a daemon that is already running holds an O_APPEND fd here,
# so renaming would leave it writing to the rotated file forever.
if [ -f "$LOG" ] && [ "$(stat -c %s "$LOG" 2>/dev/null || echo 0)" -gt "$MAX_LOG" ]; then
    tail -c 262144 "$LOG" >"$LOG.1" 2>/dev/null
    : >"$LOG"
fi

api_up() {
    grep -q ":$PORT_HEX 00000000:0000 0A" /proc/net/tcp 2>/dev/null
}

unit_active() {
    systemctl --user is-active --quiet "$1.service" 2>/dev/null
}

# start_unit NAME BINARY LOGFILE - run a bundled binary as its own user unit,
# with output still going to the app's log file so the on-device recipes and the
# UI's diagnostics keep working. --collect reaps a previously failed unit so the
# name can be reused on the next launch.
start_unit() {
    local unit="$1" bin="$2" log="$3"
    systemd-run --user --collect --quiet --unit="$unit" \
        --setenv=HOME="$HOME" \
        --setenv=XDG_RUNTIME_DIR="$RUNTIME_DIR" \
        --setenv=DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$RUNTIME_DIR/bus}" \
        --setenv=XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}" \
        --setenv=XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}" \
        /bin/sh -c "exec '$bin' >>'$log' 2>&1" >/dev/null 2>&1
}

if ! unit_active "$DAEMON_UNIT" && ! api_up; then
    echo "=== pulsed start $(date -Is) ===" >>"$LOG"
    start_unit "$DAEMON_UNIT" "$APP_DIR/bin/pulsed" "$LOG"
    # Give the API a moment so the first UI poll does not fail for nothing.
    for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
        api_up && break
        sleep 0.2
    done
fi

# Waydroid notification relay. Independent of pulsed: it only needs the session
# bus, and it is what puts Android notifications into the Lomiri shade. Skipped
# when Waydroid is not installed on the device.
if command -v waydroid >/dev/null 2>&1 && ! unit_active "$WD_UNIT"; then
    echo "=== pulse-wdnotify start $(date -Is) ===" >>"$WD_LOG"
    start_unit "$WD_UNIT" "$APP_DIR/bin/pulse-wdnotify" "$WD_LOG"
fi

cd "$APP_DIR"
exec qmlscene "$@" qml/Main.qml
