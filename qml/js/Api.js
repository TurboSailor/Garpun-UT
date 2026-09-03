.pragma library

// Sole network layer of the app. Everything goes to the local pulsed daemon
// over plain HTTP; there is no other transport and no third-party QML module
// involved, only XMLHttpRequest.

var BASE = "http://127.0.0.1:21830";

function url(path) {
    return BASE + path;
}

// Low level request. `ok(parsedBody, status)` on 2xx, `fail(message, status)`
// otherwise. A dead daemon surfaces as status 0 which every screen renders as
// an offline state rather than an error popup.
function request(method, path, body, ok, fail) {
    var xhr = new XMLHttpRequest();
    try {
        xhr.open(method, url(path));
    } catch (e) {
        if (fail) fail("" + e, 0);
        return null;
    }
    if (body !== null && body !== undefined)
        xhr.setRequestHeader("Content-Type", "application/json");
    xhr.onreadystatechange = function () {
        if (xhr.readyState !== 4 /* DONE */)
            return;
        var st = xhr.status;
        if (st >= 200 && st < 300) {
            var parsed = null;
            if (xhr.responseText && xhr.responseText.length > 0) {
                try {
                    parsed = JSON.parse(xhr.responseText);
                } catch (err) {
                    if (fail) fail("bad json: " + err, st);
                    return;
                }
            }
            if (ok) ok(parsed, st);
        } else if (fail) {
            fail(st === 0 ? "daemon unreachable" : "HTTP " + st, st);
        }
    };
    try {
        xhr.send(body === null || body === undefined ? undefined : JSON.stringify(body));
    } catch (e2) {
        if (fail) fail("" + e2, 0);
    }
    return xhr;
}

function get(path, ok, fail) { return request("GET", path, null, ok, fail); }
function post(path, body, ok, fail) { return request("POST", path, body, ok, fail); }
function put(path, body, ok, fail) { return request("PUT", path, body, ok, fail); }

function q(params) {
    var parts = [];
    for (var k in params) {
        if (params[k] === undefined || params[k] === null) continue;
        parts.push(encodeURIComponent(k) + "=" + encodeURIComponent(params[k]));
    }
    return parts.length ? "?" + parts.join("&") : "";
}

// ---- contract endpoints -------------------------------------------------

function status(ok, fail)            { return get("/api/status", ok, fail); }
function devices(ok, fail)           { return get("/api/devices", ok, fail); }
function scanStart(ms, ok, fail)     { return post("/api/scan", { durationMs: ms }, ok, fail); }
function scanResults(ok, fail)       { return get("/api/scan/results", ok, fail); }
function pair(addr, ok, fail)        { return post("/api/pair", { address: addr }, ok, fail); }
function pairingState(ok, fail)      { return get("/api/pairing", ok, fail); }
function pairingReply(body, ok, fail){ return post("/api/pairing", body, ok, fail); }
function connect(addr, ok, fail)     { return post("/api/connect", { address: addr }, ok, fail); }
function disconnect(ok, fail)        { return post("/api/disconnect", {}, ok, fail); }
function forget(addr, ok, fail)      { return post("/api/forget", { address: addr }, ok, fail); }
function sync(ok, fail)              { return post("/api/sync", {}, ok, fail); }
function today(date, ok, fail)       { return get("/api/today" + q({ date: date }), ok, fail); }
function health(days, ok, fail)      { return get("/api/health" + q({ days: days }), ok, fail); }
function sleep(date, ok, fail)       { return get("/api/sleep" + q({ date: date }), ok, fail); }
function workouts(limit, ok, fail)   { return get("/api/workouts" + q({ limit: limit }), ok, fail); }
function workout(id, ok, fail)       { return get("/api/workouts/" + id, ok, fail); }
function settingsGet(ok, fail)       { return get("/api/settings", ok, fail); }
function settingsPut(body, ok, fail) { return put("/api/settings", body, ok, fail); }
function findWatch(sec, ok, fail)    { return post("/api/findwatch", { seconds: sec }, ok, fail); }
function findWatchCancel(ok, fail)   { return post("/api/findwatch/cancel", {}, ok, fail); }

// ---- SSE ----------------------------------------------------------------

// GET /api/events is a text/event-stream. Qt's XMLHttpRequest exposes the
// partial body at readyState 3, so we parse incrementally from a cursor and
// never buffer the whole (unbounded) stream twice.
function EventStream(onEvent, onState) {
    this._xhr = null;
    this._cursor = 0;
    this._closed = false;
    this.onEvent = onEvent;
    this.onState = onState;
}

EventStream.prototype.open = function () {
    if (this._closed) return;
    var self = this;
    var xhr = new XMLHttpRequest();
    this._xhr = xhr;
    this._cursor = 0;
    xhr.onreadystatechange = function () {
        if (self._closed) return;
        if (xhr.readyState === 3 /* LOADING */) {
            if (self.onState) self.onState(true);
            self._drain(xhr.responseText);
        } else if (xhr.readyState === 4 /* DONE */) {
            self._drain(xhr.responseText);
            self._xhr = null;
            if (self.onState) self.onState(false);
        }
    };
    try {
        xhr.open("GET", url("/api/events"));
        xhr.setRequestHeader("Accept", "text/event-stream");
        xhr.send();
    } catch (e) {
        this._xhr = null;
        if (this.onState) this.onState(false);
    }
};

EventStream.prototype._drain = function (text) {
    if (!text) return;
    // Only complete lines are consumable; keep the tail for the next chunk.
    var lastNl = text.lastIndexOf("\n");
    if (lastNl < this._cursor) return;
    var chunk = text.substring(this._cursor, lastNl + 1);
    this._cursor = lastNl + 1;
    var lines = chunk.split("\n");
    for (var i = 0; i < lines.length; i++) {
        var line = lines[i];
        if (line.substring(0, 5) !== "data:") continue;
        var payload = line.substring(5).replace(/^\s+/, "");
        if (!payload.length) continue;
        var obj;
        try {
            obj = JSON.parse(payload);
        } catch (e) {
            continue;
        }
        if (this.onEvent) this.onEvent(obj);
    }
};

EventStream.prototype.connected = function () {
    return this._xhr !== null;
};

EventStream.prototype.close = function () {
    this._closed = true;
    if (this._xhr) {
        try { this._xhr.abort(); } catch (e) {}
        this._xhr = null;
    }
};
