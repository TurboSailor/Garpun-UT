.pragma library
.import "I18n.js" as I18n

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
    var isRu = I18n.isRu();
    var kmU = isRu ? " км" : " km";
    var mU = isRu ? " м" : " m";
    var miU = isRu ? " ми" : " mi";
    if (unitSystem === "imperial") {
        var mi = metres / 1609.344;
        return (mi < 10 ? mi.toFixed(2) : mi.toFixed(1)) + miU;
    }
    if (metres < 1000) return Math.round(metres) + mU;
    var km = metres / 1000;
    return (km < 10 ? km.toFixed(2) : km.toFixed(1)) + kmU;
}

function distanceShort(metres, unitSystem) {
    if (!has(metres)) return EMPTY;
    if (unitSystem === "imperial") return (metres / 1609.344).toFixed(1);
    return (metres / 1000).toFixed(metres < 10000 ? 2 : 1);
}

function distanceUnit(unitSystem) {
    var isRu = I18n.isRu();
    if (unitSystem === "imperial") return isRu ? "ми" : "mi";
    return isRu ? "км" : "km";
}

// minutes -> "7h 12m" / "48m" (or "7 ч 12 мин" in RU)
function duration(mins) {
    if (!has(mins) || mins <= 0) return EMPTY;
    var isRu = I18n.isRu();
    var h = Math.floor(mins / 60);
    var m = Math.round(mins % 60);
    var hU = isRu ? " ч " : "h ";
    var mU = isRu ? " мин" : "m";
    if (h <= 0) return m + mU;
    return h + hU + (m > 0 ? m + mU : "");
}

function durationTrim(mins) {
    var s = duration(mins);
    return s.replace(/\s+$/, "");
}

// m/s -> "5:24 /km" (or /mi). Running and walking are read as pace, not speed.
function pace(mps, unitSystem) {
    if (!has(mps) || mps <= 0.15) return EMPTY;
    var perUnit = (unitSystem === "imperial" ? 1609.344 : 1000) / mps;
    var m = Math.floor(perUnit / 60);
    var s = Math.round(perUnit % 60);
    if (s === 60) { m += 1; s = 0; }
    if (m > 99) return EMPTY;
    return m + ":" + pad2(s) + " /" + distanceUnit(unitSystem);
}

// m/s -> minutes per unit, for charting where a number is needed.
function paceMinutes(mps, unitSystem) {
    if (!has(mps) || mps <= 0.15) return 0;
    return (unitSystem === "imperial" ? 1609.344 : 1000) / mps / 60;
}

function isFootSport(sport) {
    return sport === 1 || sport === 11 || sport === 17;
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

var MONTHS_EN = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
var MONTHS_RU = ["янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];

var DAYS_EN = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
var DAYS_RU = ["Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"];

var DAYS_SHORT_EN = ["S", "M", "T", "W", "T", "F", "S"];
var DAYS_SHORT_RU = ["Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"];

function months() {
    return I18n.isRu() ? MONTHS_RU : MONTHS_EN;
}

function days() {
    return I18n.isRu() ? DAYS_RU : DAYS_EN;
}

function daysShort() {
    return I18n.isRu() ? DAYS_SHORT_RU : DAYS_SHORT_EN;
}

function prettyDate(iso) {
    if (!has(iso)) return "";
    var d = parseIso(iso);
    var now = new Date();
    if (isoDate(now) === iso) return I18n.t("date.today");
    now.setDate(now.getDate() - 1);
    if (isoDate(now) === iso) return I18n.t("date.yesterday");
    return days()[d.getDay()] + ", " + d.getDate() + " " + months()[d.getMonth()];
}

function timeOfDay(ms) {
    if (!has(ms) || ms <= 0) return EMPTY;
    var d = new Date(ms);
    return pad2(d.getHours()) + ":" + pad2(d.getMinutes());
}

function dayShort(ms) {
    if (!has(ms) || ms <= 0) return "";
    return daysShort()[new Date(ms).getDay()];
}

function dateShort(ms) {
    if (!has(ms) || ms <= 0) return EMPTY;
    var d = new Date(ms);
    return d.getDate() + " " + months()[d.getMonth()];
}

function relative(ms) {
    if (!has(ms) || ms <= 0) return I18n.t("date.never");
    var diff = Date.now() - ms;
    if (diff < 0) return I18n.t("date.just_now");
    var mins = Math.floor(diff / 60000);
    if (mins < 1) return I18n.t("date.just_now");
    if (mins < 60) return I18n.t("date.mins_ago", [mins]);
    var hrs = Math.floor(mins / 60);
    if (hrs < 24) return I18n.t("date.hours_ago", [hrs]);
    var daysCount = Math.floor(hrs / 24);
    if (daysCount < 7) return I18n.t("date.days_ago", [daysCount]);
    return dateShort(ms);
}

function greeting(hour) {
    if (hour < 12) return I18n.t("greeting.morning");
    if (hour < 18) return I18n.t("greeting.afternoon");
    return I18n.t("greeting.evening");
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
    case 0x10: return I18n.t("sport.run");
    case 0x20: return I18n.t("sport.walk");
    case 0x40: return I18n.t("sport.swim");
    case 0x80: return I18n.t("sport.ride");
    case 0x100: return I18n.t("sport.treadmill");
    case 0x200: return I18n.t("sport.exercise");
    case 0x1: return I18n.t("sport.activity");
    case 0x04000000: return I18n.t("sport.navigate");
    case 0x04000001: return I18n.t("sport.indoor_track");
    }
    return I18n.t("sport.workout");
}

// FIT sport enum -> label, covers what a Forerunner actually records.
function sportName(sport) {
    switch (sport) {
    case 0: return I18n.t("sport.generic");
    case 1: return I18n.t("sport.run");
    case 2: return I18n.t("sport.ride");
    case 4: return I18n.t("sport.swim");
    case 5: return I18n.t("sport.basketball");
    case 6: return I18n.t("sport.soccer");
    case 7: return I18n.t("sport.tennis");
    case 9: return I18n.t("sport.row");
    case 11: return I18n.t("sport.walk");
    case 13: return I18n.t("sport.alpine_ski");
    case 15: return I18n.t("sport.row");
    case 17: return I18n.t("sport.hike");
    case 18: return I18n.t("sport.multisport");
    case 25: return I18n.t("sport.strength");
    case 26: return I18n.t("sport.cardio");
    case 41: return I18n.t("sport.paddling");
    case 43: return I18n.t("sport.yoga");
    }
    return I18n.t("sport.workout");
}

function workoutTitle(w) {
    if (!w) return EMPTY;
    if (has(w.name)) return w.name;
    if (has(w.sport)) return sportName(w.sport);
    return activityName(w.kind);
}
