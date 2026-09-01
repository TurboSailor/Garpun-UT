#!/usr/bin/env bash
# Install a built .click on the phone and verify the click/apparmor registration.
# Usage: scripts/deploy.sh [CLICK]   (PULSE_SUDO_PASS overrides the sudo password)
set -euo pipefail

CLICK="${1:-}"
PASS="${PULSE_SUDO_PASS:-googooshasha}"
REMOTE_DIR="/home/phablet/Downloads"

ADB=(adb)
[ -n "${ADB_SERIAL:-}" ] && ADB=(adb -s "$ADB_SERIAL")

die() { echo "deploy.sh: $*" >&2; exit 1; }
# Every adb shell is a fresh session, so sudo never has a cached credential.
sudo_sh() { "${ADB[@]}" shell "echo '$PASS' | sudo -S $1 2>&1" | tr -d '\r'; }

if [ -z "$CLICK" ]; then
    CLICK="$(ls -t build/*.click 2>/dev/null | head -n1 || true)"
    [ -n "$CLICK" ] || die "no .click in build/ (run: make click)"
fi
[ -f "$CLICK" ] || die "$CLICK not found"
"${ADB[@]}" get-state >/dev/null 2>&1 || die "no adb device"

base="$(basename "$CLICK")"
echo ">> pushing $base"
"${ADB[@]}" push "$CLICK" "$REMOTE_DIR/$base" >/dev/null

echo ">> click install"
# debsig signatures are absent for locally built packages -> allow-unauthenticated.
out="$(sudo_sh "click install --force --allow-unauthenticated --user=phablet $REMOTE_DIR/$base")"
echo "$out"

pkg="$("${ADB[@]}" shell "click list 2>/dev/null" | tr -d '\r' | sed -n 's/^cc\.zachy\.pulse\t.*/&/p')"
if [ -z "$pkg" ]; then
    echo "!! cc.zachy.pulse is not in click list; leftovers:"
    sudo_sh "ls -d /opt/click.ubuntu.com/cc.zachy.pulse 2>/dev/null"
    die "install failed (clean up with: sudo rm -rf /opt/click.ubuntu.com/cc.zachy.pulse)"
fi
echo ">> click list: $pkg"

prof="$(sudo_sh "aa-status" | sed -n '/cc\.zachy\.pulse/p' | head -n3)"
if [ -z "$prof" ]; then
    echo ">> apparmor profile missing, regenerating hooks"
    sudo_sh "aa-clickhook -f"
    prof="$(sudo_sh "aa-status" | sed -n '/cc\.zachy\.pulse/p' | head -n3)"
fi
[ -n "$prof" ] || die "apparmor profile for cc.zachy.pulse was not generated"
echo ">> aa-status:"
echo "$prof"

ver="$(printf '%s' "$pkg" | awk '{print $2}')"
echo ">> app id: cc.zachy.pulse_pulse_$ver  (tap the icon; adb launches lack a trust session)"
