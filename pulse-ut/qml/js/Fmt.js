.pragma library

var EMPTY = "\u2013"; // stats_empty_value

function has(v) {
    return v !== undefined && v !== null && v !== "" && !(typeof v === "number" && isNaN(v));
}

function num(v, fallback) {
    return has(v) ? v : (fallback === undefined ? 0 : fallback);
}

function thousands(n) {
    if (!has(n)) return EMPTY;
    var s = Math.round(n).toString();
    var out = "";
    var c = 0;
    for (var i = s.length - 1; i >= 0; i--) {
        out = s.charAt(i) + out;
        if (++c % 3 === 0 && i > 0 && s.charAt(i - 1) !== "-") out = "\u2009" + out;
    }
    return out;
}

// metres -> "5.2 km" / "3.2 mi"
function distance(metres, unitSystem) {
    if (!has(metres)) return EMPTY;
    if (unitSystem === "imperial") {
        var mi = metres / 1609.344;
        return (mi < 10 ? mi.toFixed(2) : mi.toFixed(1)) + " mi";
    }
    if (metres < 1000) return Math.round(metres) + " m";
    var km = metres / 1000;
    return (km < 10 ? km.toFixed(2) : km.toFixed(1)) + " km";
}

function distanceShort(metres, unitSystem) {
    if (!has(metres)) return EMPTY;
    if (unitSystem === "imperial") return (metres / 1609.344).toFixed(1);
    return (metres / 1000).toFixed(metres < 10000 ? 2 : 1);
}

function distanceUnit(unitSystem) {
    return unitSystem === "imperial" ? "mi" : "km";
}

// minutes -> "7h 12m" / "48m"
function duration(mins) {
    if (!has(mins) || mins <= 0) return EMPTY;
    var h = Math.floor(mins / 60);
    var m = Math.round(mins % 60);
    if (h <= 0) return m + "m";
    return h + "h " + (m > 0 ? m + "m" : "");
}

function durationTrim(mins) {
    var s = duration(mins);
    return s.replace(/\s+$/, "");
}

// seconds -> "1:04:22" / "4:22"
function clock(secs) {
    if (!has(secs) || secs < 0) return EMPTY;
    var h = Math.floor(secs / 3600);
    var m = Math.floor((secs % 3600) / 60);
    var s = Math.floor(secs % 60);
    var mm = (m < 10 && h > 0) ? "0" + m : "" + m;
    var ss = s < 10 ? "0" + s : "" + s;
    return (h > 0 ? h + ":" : "") + mm + ":" + ss;
}

function pad2(n) {
    return n < 10 ? "0" + n : "" + n;
}

function isoDate(d) {
    return d.getFullYear() + "-" + pad2(d.getMonth() + 1) + "-" + pad2(d.getDate());
}

function todayIso() {
    return isoDate(new Date());
}

function parseIso(s) {
    var p = ("" + s).split("-");
    return new Date(parseInt(p[0], 10), parseInt(p[1], 10) - 1, parseInt(p[2], 10));
}

function shiftIso(s, days) {
    var d = parseIso(s);
    d.setDate(d.getDate() + days);
    return isoDate(d);
}

var MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
var DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
var DAYS_SHORT = ["S", "M", "T", "W", "T", "F", "S"];

function prettyDate(iso) {
    if (!has(iso)) return "";
    var d = parseIso(iso);
    var now = new Date();
    if (isoDate(now) === iso) return "Today";
    now.setDate(now.getDate() - 1);
    if (isoDate(now) === iso) return "Yesterday";
    return DAYS[d.getDay()] + ", " + MONTHS[d.getMonth()] + " " + d.getDate();
}

function timeOfDay(ms) {
    if (!has(ms) || ms <= 0) return EMPTY;
    var d = new Date(ms);
    return pad2(d.getHours()) + ":" + pad2(d.getMinutes());
}

function dayShort(ms) {
    if (!has(ms) || ms <= 0) return "";
    return DAYS_SHORT[new Date(ms).getDay()];
}

function dateShort(ms) {
    if (!has(ms) || ms <= 0) return EMPTY;
    var d = new Date(ms);
    return MONTHS[d.getMonth()] + " " + d.getDate();
}

function relative(ms) {
    if (!has(ms) || ms <= 0) return "never";
    var diff = Date.now() - ms;
    if (diff < 0) return "just now";
    var mins = Math.floor(diff / 60000);
    if (mins < 1) return "just now";
    if (mins < 60) return mins + " min ago";
    var hrs = Math.floor(mins / 60);
    if (hrs < 24) return hrs + "h ago";
    var days = Math.floor(hrs / 24);
    if (days < 7) return days + "d ago";
    return dateShort(ms);
}

function greeting(hour) {
    if (hour < 12) return "Good morning";
    if (hour < 18) return "Good afternoon";
    return "Good evening";
}

function signed(v, digits) {
    if (!has(v)) return "";
    var d = digits === undefined ? 0 : digits;
    var s = Math.abs(v).toFixed(d);
    if (v > 0) return "+" + s;
    if (v < 0) return "\u2212" + s;
    return s;
}

function trimNum(v, digits) {
    if (!has(v)) return EMPTY;
    var d = digits === undefined ? 0 : digits;
    var s = v.toFixed(d);
    if (d > 0) s = s.replace(/\.?0+$/, "");
    return s;
}

// Garmin ActivityKind -> readable label (docs §1.5 code table).
function activityName(kind) {
    switch (kind) {
    case 0x10: return "Run";
    case 0x20: return "Walk";
    case 0x40: return "Swim";
    case 0x80: return "Ride";
    case 0x100: return "Treadmill";
    case 0x200: return "Exercise";
    case 0x1: return "Activity";
    case 0x04000000: return "Navigate";
    case 0x04000001: return "Indoor track run";
    }
    return "Workout";
}

// FIT sport enum -> label, covers what a Forerunner actually records.
function sportName(sport) {
    switch (sport) {
    case 0: return "Generic";
    case 1: return "Run";
    case 2: return "Ride";
    case 4: return "Swim";
    case 5: return "Basketball";
    case 6: return "Soccer";
    case 7: return "Tennis";
    case 9: return "Row";
    case 11: return "Walk";
    case 13: return "Alpine ski";
    case 15: return "Row";
    case 17: return "Hike";
    case 18: return "Multisport";
    case 25: return "Strength";
    case 26: return "Cardio";
    case 41: return "Paddling";
    case 43: return "Yoga";
    }
    return "Workout";
}

function workoutTitle(w) {
    if (!w) return EMPTY;
    if (has(w.name)) return w.name;
    if (has(w.sport)) return sportName(w.sport);
    return activityName(w.kind);
}
