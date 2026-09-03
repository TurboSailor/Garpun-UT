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

pkg="$("${ADB[@]}" shell "click list 2>/dev/null" | tr -d '\r' | sed -n 's/^pulse\.turbosailor\t.*/&/p')"
if [ -z "$pkg" ]; then
    echo "!! pulse.turbosailor is not in click list; leftovers:"
    sudo_sh "ls -d /opt/click.ubuntu.com/pulse.turbosailor 2>/dev/null"
    die "install failed (clean up with: sudo rm -rf /opt/click.ubuntu.com/pulse.turbosailor)"
fi
echo ">> click list: $pkg"

prof="$(sudo_sh "aa-status" | sed -n '/pulse\.turbosailor/p' | head -n3)"
if [ -z "$prof" ]; then
    echo ">> apparmor profile missing, regenerating hooks"
    sudo_sh "aa-clickhook -f"
    prof="$(sudo_sh "aa-status" | sed -n '/pulse\.turbosailor/p' | head -n3)"
fi
[ -n "$prof" ] || die "apparmor profile for pulse.turbosailor was not generated"
echo ">> aa-status:"
echo "$prof"

# The daemon keeps running out of the previous unpack directory: `current`
# already points at the new one, but the live process holds the old binary.
# Restart its unit so a redeploy actually takes effect. The unit exists only
# after the app has been opened once - run.sh is what creates it.
echo ">> restarting pulse-pulsed.service"
"${ADB[@]}" shell "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/32011/bus \
    systemctl --user try-restart pulse-pulsed.service" >/dev/null 2>&1 \
    || echo "!! could not restart pulse-pulsed; open the app to start it"

ver="$(printf '%s' "$pkg" | awk '{print $2}')"
echo ">> app id: pulse.turbosailor_pulse_$ver  (tap the icon; adb launches lack a trust session)"
