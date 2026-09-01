#!/bin/bash
# Click entry point: bring up the pulsed daemon (once), then hand over to the UI.
# The daemon lives in the app confinement, so it dies with the session; the UI
# talks to it over http://127.0.0.1:21830 only.
set -u

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/pulse"
PIDFILE="$RUNTIME_DIR/pulsed.pid"
LOG="$CACHE_DIR/pulsed.log"
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

daemon_alive() {
    if [ -f "$PIDFILE" ]; then
        local pid
        pid="$(cat "$PIDFILE" 2>/dev/null)"
        if [ -n "$pid" ] && [ -r "/proc/$pid/comm" ] && grep -q '^pulsed$' "/proc/$pid/comm"; then
            return 0
        fi
    fi
    api_up
}

if ! daemon_alive; then
    echo "=== pulsed start $(date -Is) ===" >>"$LOG"
    # Own session: the daemon must outlive the launching shell (SIGHUP) and stay
    # up while the UI is restarted. setsid execs in place, so $! is pulsed itself.
    setsid "$APP_DIR/bin/pulsed" >>"$LOG" 2>&1 </dev/null &
    echo $! >"$PIDFILE"
    # Give the API a moment so the first UI poll does not fail for nothing.
    for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
        api_up && break
        sleep 0.2
    done
fi

cd "$APP_DIR"
exec qmlscene "$@" qml/Main.qml
