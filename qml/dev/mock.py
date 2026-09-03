#!/usr/bin/env python3
"""Fixture server for the Pulse QML frontend.

Speaks exactly the contract in the project brief so the UI can be exercised on
the device without pulsed, a watch or Bluetooth. Run it on the phone:

    python3 /home/phablet/pulse-qml/dev/mock.py

Options: --port (default 21830), --empty (every endpoint answers with the
"no data yet" shape, to check skeletons and empty states).
"""

import argparse
import json
import math
import random
import threading
import time
from datetime import date, datetime, timedelta
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs

PORT = 21830
EMPTY = False

STATE = {
    "scanning": False,
    "scan": [],
    "pairing": {"pending": False, "kind": "", "address": "", "passkey": 0},
    "syncing": False,
    "progress": {"fileIndex": 0, "received": 0, "total": 0, "remaining": 0},
    "connected": True,
    "lastSyncMs": int(time.time() * 1000) - 18 * 60 * 1000,
}

SETTINGS = {
    "theme": "dark",
    "accent": "blue",
    "stepsGoal": 10000,
    "sleepGoalMinutes": 480,
    "activeCaloriesGoal": 350,
    "distanceGoalM": 5000,
    "activeMinutesGoal": 60,
    "intensityGoal": 30,
    "units": "metric",
    "syncTime": True,
    "weatherEnabled": True,
    "notificationsEnabled": True,
    "notifyWaydroid": True,
    "keepFilesOnWatch": False,
    "anyGoalStreak": False,
    "autoSyncMinutes": 60,
}

SUBSCRIBERS = []
SUB_LOCK = threading.Lock()


def ms(dt):
    return int(dt.timestamp() * 1000)


def broadcast(kind, data=None):
    line = "data: " + json.dumps({"kind": kind, "data": data}) + "\n\n"
    payload = line.encode()
    with SUB_LOCK:
        dead = []
        for w in SUBSCRIBERS:
            try:
                w.write(payload)
                w.flush()
            except Exception:
                dead.append(w)
        for w in dead:
            SUBSCRIBERS.remove(w)


# --------------------------------------------------------------------------
# fixtures
# --------------------------------------------------------------------------

def status():
    if EMPTY:
        return {
            "daemonVersion": "0.0.1-mock",
            "adapterPowered": True,
            "device": None,
            "syncing": False,
            "progress": {"fileIndex": 0, "received": 0, "total": 0, "remaining": 0},
        }
    return {
        "daemonVersion": "0.0.1-mock",
        "adapterPowered": True,
        "device": {
            "address": "D4:F0:57:11:22:33",
            "name": "Forerunner 255",
            "model": "Forerunner 255",
            "firmware": "18.22",
            "connected": STATE["connected"],
            "initialized": STATE["connected"],
            "batteryLevel": 74,
            "lastSyncMs": STATE["lastSyncMs"],
        },
        "syncing": STATE["syncing"],
        "progress": STATE["progress"],
    }


def devices():
    if EMPTY:
        return []
    return [{
        "address": "D4:F0:57:11:22:33",
        "name": "Forerunner 255",
        "model": "Forerunner 255",
        "paired": True,
        "connected": STATE["connected"],
        "lastSyncMs": STATE["lastSyncMs"],
    }]


def today(day):
    if EMPTY:
        return {
            "date": day, "steps": 0, "stepsGoal": SETTINGS["stepsGoal"], "distanceM": 0.0,
            "activeCalories": 0, "restingCalories": 0, "totalCalories": 0, "activeMinutes": 0,
            "heartRate": {"latest": 0, "resting": 0, "min": 0, "max": 0},
            "bodyEnergy": {"latest": 0, "min": 0, "max": 0},
            "stress": {"latest": 0, "avg": 0},
            "spo2": {"latest": 0},
            "respiration": {"latest": 0.0},
            "sleepMinutes": 0, "sleepScore": 0,
            "intensityMinutes": {"today": 0, "week": 0, "goal": SETTINGS["intensityGoal"]},
            "streak": {"current": 0, "best": 0},
            "goals": goals(),
        }
    seed = sum(ord(c) for c in day)
    rnd = random.Random(seed)
    steps = 7400 + rnd.randint(0, 6200)
    return {
        "date": day,
        "steps": steps,
        "stepsGoal": SETTINGS["stepsGoal"],
        "distanceM": round(steps * 0.74, 1),
        "activeCalories": 280 + rnd.randint(0, 260),
        "restingCalories": 1580,
        "totalCalories": 1900 + rnd.randint(0, 260),
        "activeMinutes": 38 + rnd.randint(0, 55),
        "heartRate": {"latest": 62 + rnd.randint(0, 20), "resting": 48 + rnd.randint(0, 6),
                      "min": 44, "max": 158},
        "bodyEnergy": {"latest": 40 + rnd.randint(0, 50), "min": 22, "max": 96},
        "stress": {"latest": 18 + rnd.randint(0, 40), "avg": 31},
        "spo2": {"latest": 95 + rnd.randint(0, 3)},
        "respiration": {"latest": round(13 + rnd.random() * 3, 1)},
        "sleepMinutes": 380 + rnd.randint(0, 120),
        "sleepScore": 62 + rnd.randint(0, 32),
        "intensityMinutes": {"today": 12 + rnd.randint(0, 45), "week": 140 + rnd.randint(0, 90),
                             "goal": SETTINGS["intensityGoal"]},
        "streak": {"current": 6, "best": 23},
        "goals": goals(),
    }


def goals():
    return {
        "steps": SETTINGS["stepsGoal"],
        "sleepMinutes": SETTINGS["sleepGoalMinutes"],
        "activeCalories": SETTINGS["activeCaloriesGoal"],
        "distanceM": SETTINGS["distanceGoalM"],
        "activeMinutes": SETTINGS["activeMinutesGoal"],
        "intensityMinutes": SETTINGS["intensityGoal"],
    }


METRICS = [
    ("body_energy", "Body Battery", "", 30, 95, 0),
    ("heart_rate", "Heart rate", "bpm", 52, 88, 0),
    ("stress", "Stress", "", 12, 55, 0),
    ("spo2", "Blood oxygen", "%", 93, 99, 0),
    ("hrv", "HRV", "ms", 38, 78, 1),
    ("respiration", "Respiration", "br/min", 11, 17, 1),
    ("resting_hr", "Resting HR", "bpm", 45, 55, 0),
]


def health(days):
    if EMPTY:
        return {"metrics": []}
    out = []
    now = datetime.now()
    for key, label, unit, lo, hi, dec in METRICS:
        rnd = random.Random(hash(key) & 0xFFFF)
        series = []
        for i in range(days):
            ts = now - timedelta(days=days - 1 - i)
            v = lo + (hi - lo) * (0.5 + 0.5 * math.sin(i / 2.0 + rnd.random()))
            series.append({"tsMs": ms(ts), "value": round(v, dec)})
        latest = series[-1]["value"]
        prev = sum(p["value"] for p in series[:-1]) / max(1, len(series) - 1)
        out.append({
            "key": key, "label": label, "unit": unit,
            "latest": latest, "delta": round(latest - prev, dec),
            "series": series,
        })
    return {"metrics": out}


def sleep(day):
    if EMPTY:
        return {"score": 0, "quality": "", "startMs": 0, "endMs": 0,
                "totals": {"deep": 0, "light": 0, "rem": 0, "awake": 0},
                "stages": [], "naps": [], "trend": [], "restlessMoments": 0}
    d = datetime.strptime(day, "%Y-%m-%d")
    start = d.replace(hour=0, minute=0, second=0) - timedelta(hours=1, minutes=20)
    stages = []
    cursor = start
    plan = [("light", 22), ("deep", 48), ("light", 30), ("rem", 26), ("light", 18),
            ("awake", 6), ("deep", 34), ("light", 26), ("rem", 40), ("light", 20),
            ("awake", 4), ("light", 24), ("rem", 28), ("light", 32)]
    totals = {"deep": 0, "light": 0, "rem": 0, "awake": 0}
    for stage, minutes in plan:
        end = cursor + timedelta(minutes=minutes)
        stages.append({"startMs": ms(cursor), "endMs": ms(end), "stage": stage})
        totals[stage] += minutes
        cursor = end
    asleep = totals["deep"] + totals["light"] + totals["rem"]
    score = 78
    trend = []
    for i in range(7):
        dd = d - timedelta(days=6 - i)
        rnd = random.Random(dd.toordinal())
        mins = 0 if i == 2 else 360 + rnd.randint(0, 130)
        trend.append({"date": dd.strftime("%Y-%m-%d"), "minutes": mins,
                      "score": 0 if mins == 0 else 55 + rnd.randint(0, 40)})
    nap_start = d.replace(hour=14, minute=10, second=0)
    return {
        "score": score,
        "quality": "Good",
        "startMs": ms(start),
        "endMs": ms(cursor),
        "totals": totals,
        "stages": stages,
        "naps": [{"startMs": ms(nap_start), "endMs": ms(nap_start + timedelta(minutes=28)),
                  "minutes": 28}],
        "trend": trend,
        "restlessMoments": 14,
        "asleepMinutes": asleep,
    }


def workout_list():
    if EMPTY:
        return []
    out = []
    now = datetime.now()
    plan = [(1, "Morning run", 46, 8300), (11, "Evening walk", 62, 4200),
            (2, "Commute ride", 34, 12400), (25, "Strength", 41, 0),
            (1, "Interval session", 52, 9600)]
    for i, (sport, name, minutes, dist) in enumerate(plan):
        start = now - timedelta(days=i * 2 + 1, hours=3)
        end = start + timedelta(minutes=minutes)
        out.append({
            "id": 100 + i,
            "startMs": ms(start),
            "endMs": ms(end),
            "kind": 0x10 if sport == 1 else 0x20,
            "sport": sport,
            "name": name,
            "summary": {
                "distanceM": dist,
                "activeCalories": 220 + i * 40,
                "avgHeartRate": 138 + i * 3,
                "maxHeartRate": 168 + i,
                "avgCadence": 168 if sport == 1 else 82,
                "totalAscentM": 40 + i * 12,
                "avgSpeed": round(dist / (minutes * 60), 2) if dist else 0,
            },
        })
    return out


def workout(wid):
    for w in workout_list():
        if w["id"] == wid:
            detail = dict(w)
            n = 240
            track = []
            t0 = w["startMs"]
            step = (w["endMs"] - w["startMs"]) / n
            for i in range(n):
                f = i / float(n)
                track.append({
                    "tsMs": int(t0 + i * step),
                    "lat": 55.751 + 0.010 * math.sin(f * 6.0) + 0.004 * f,
                    "lon": 37.618 + 0.014 * math.cos(f * 5.0),
                    "altitude": 140 + 22 * math.sin(f * 9.0),
                    "heartRate": int(126 + 26 * math.sin(f * 7.0)),
                    "cadence": int(160 + 12 * math.sin(f * 11.0)),
                    "speed": round(2.9 + 0.9 * math.sin(f * 4.0), 2),
                    "power": int(230 + 40 * math.sin(f * 5.0)),
                    "distance": round(w["summary"]["distanceM"] * f, 1),
                })
            detail["track"] = track
            return detail
    return None


# --------------------------------------------------------------------------
# background jobs
# --------------------------------------------------------------------------

def run_scan(duration_ms):
    STATE["scanning"] = True
    STATE["scan"] = []
    found = [
        {"address": "D4:F0:57:11:22:33", "name": "Forerunner 255", "rssi": -52,
         "paired": True, "garmin": True},
        {"address": "C8:1A:33:90:AB:01", "name": "Instinct 2", "rssi": -74,
         "paired": False, "garmin": True},
        {"address": "44:21:8C:0F:2E:9D", "name": "", "rssi": -91,
         "paired": False, "garmin": False},
        {"address": "10:2B:41:73:C6:55", "name": "JBL Go 3", "rssi": -66,
         "paired": False, "garmin": False},
    ]
    for entry in found:
        time.sleep(1.4)
        if not STATE["scanning"]:
            return
        STATE["scan"].append(entry)
        broadcast("scan_result", entry)
    time.sleep(max(0, duration_ms / 1000.0 - len(found) * 1.4))
    STATE["scanning"] = False


def run_pairing(address):
    time.sleep(1.0)
    STATE["pairing"] = {"pending": True, "kind": "passkey", "address": address,
                        "passkey": 0}
    broadcast("pairing_request", STATE["pairing"])


def run_sync():
    STATE["syncing"] = True
    broadcast("sync_started", None)
    files = 6
    for i in range(files):
        total = 40000 + i * 9000
        received = 0
        while received < total:
            received = min(total, received + 9000)
            STATE["progress"] = {"fileIndex": i, "received": received,
                                 "total": total, "remaining": files - i - 1}
            broadcast("sync_progress", STATE["progress"])
            time.sleep(0.12)
    STATE["syncing"] = False
    STATE["lastSyncMs"] = int(time.time() * 1000)
    STATE["progress"] = {"fileIndex": 0, "received": 0, "total": 0, "remaining": 0}
    broadcast("sync_finished", None)


# --------------------------------------------------------------------------

class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    def _json(self, obj, code=200):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _accepted(self):
        self.send_response(202)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def _body(self):
        n = int(self.headers.get("Content-Length") or 0)
        if n <= 0:
            return {}
        try:
            return json.loads(self.rfile.read(n).decode())
        except Exception:
            return {}

    def do_GET(self):
        u = urlparse(self.path)
        q = parse_qs(u.query)
        path = u.path

        if path == "/api/status":
            return self._json(status())
        if path == "/api/devices":
            return self._json(devices())
        if path == "/api/scan/results":
            return self._json(STATE["scan"])
        if path == "/api/pairing":
            return self._json(STATE["pairing"])
        if path == "/api/today":
            return self._json(today(q.get("date", [date.today().isoformat()])[0]))
        if path == "/api/health":
            return self._json(health(int(q.get("days", ["7"])[0])))
        if path == "/api/sleep":
            return self._json(sleep(q.get("date", [date.today().isoformat()])[0]))
        if path == "/api/workouts":
            return self._json(workout_list()[: int(q.get("limit", ["50"])[0])])
        if path.startswith("/api/workouts/"):
            w = workout(int(path.rsplit("/", 1)[1]))
            return self._json(w) if w else self._json({"error": "not found"}, 404)
        if path == "/api/settings":
            return self._json(SETTINGS)
        if path == "/api/events":
            return self._events()
        self._json({"error": "no route"}, 404)

    def _events(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.end_headers()
        with SUB_LOCK:
            SUBSCRIBERS.append(self.wfile)
        try:
            while True:
                time.sleep(15)
                self.wfile.write(b": ping\n\n")
                self.wfile.flush()
        except Exception:
            pass
        finally:
            with SUB_LOCK:
                if self.wfile in SUBSCRIBERS:
                    SUBSCRIBERS.remove(self.wfile)

    def do_POST(self):
        path = urlparse(self.path).path
        body = self._body()

        if path == "/api/scan":
            threading.Thread(target=run_scan,
                             args=(body.get("durationMs", 20000),), daemon=True).start()
            return self._accepted()
        if path == "/api/pair":
            threading.Thread(target=run_pairing,
                             args=(body.get("address", ""),), daemon=True).start()
            return self._accepted()
        if path == "/api/pairing":
            STATE["pairing"] = {"pending": False, "kind": "", "address": "", "passkey": 0}
            return self._json({"ok": True})
        if path == "/api/connect":
            STATE["connected"] = True
            broadcast("initialized", None)
            return self._json({"ok": True})
        if path == "/api/disconnect":
            STATE["connected"] = False
            broadcast("disconnected", None)
            return self._json({"ok": True})
        if path == "/api/forget":
            return self._json({"ok": True})
        if path == "/api/sync":
            threading.Thread(target=run_sync, daemon=True).start()
            return self._accepted()
        if path == "/api/findwatch":
            return self._json({"ok": True})
        if path == "/api/findwatch/cancel":
            return self._json({"ok": True})
        self._json({"error": "no route"}, 404)

    def do_PUT(self):
        if urlparse(self.path).path == "/api/settings":
            SETTINGS.update(self._body())
            broadcast("settings_changed", SETTINGS)
            return self._json(SETTINGS)
        self._json({"error": "no route"}, 404)


def main():
    global PORT, EMPTY
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=21830)
    ap.add_argument("--empty", action="store_true")
    args = ap.parse_args()
    PORT = args.port
    EMPTY = args.empty
    srv = ThreadingHTTPServer(("127.0.0.1", PORT), Handler)
    srv.daemon_threads = True
    print("mock pulsed on http://127.0.0.1:%d (empty=%s)" % (PORT, EMPTY), flush=True)
    srv.serve_forever()


if __name__ == "__main__":
    main()
