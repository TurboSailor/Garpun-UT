pragma Singleton
import QtQuick 2.12
import Ubuntu.Components 1.3

// Pulse design tokens. Palette values are lifted verbatim from the Android
// app (res/values/colors.xml + values-night/colors.xml) so the port keeps the
// original identity: near-black blue surfaces, one neon accent, no gradients
// used as decoration.
QtObject {
    id: pulse

    // ---- mode ---------------------------------------------------------
    // "system" follows the Lomiri theme (Main.qml feeds systemDark in).
    property string mode: "dark"
    property bool systemDark: true
    readonly property bool dark: mode === "dark" || (mode !== "light" && systemDark)

    property string accentName: "blue"

    // ---- surfaces -----------------------------------------------------
    readonly property color bg:      dark ? "#07070A" : "#F4F5F7"
    readonly property color card:    dark ? "#0E0E16" : "#FFFFFF"
    readonly property color cardAlt: dark ? "#17171F" : "#E4E7EC"
    readonly property color hairline: dark ? Qt.rgba(1, 1, 1, 0.06) : Qt.rgba(0, 0, 0, 0.08)

    // ---- ink ----------------------------------------------------------
    readonly property color text:    dark ? "#ECEBE6" : "#16181D"
    readonly property color textDim: dark ? "#8A8A93" : "#6B6F78"
    readonly property color onAccent: dark ? "#07070A" : "#FFFFFF"

    // ---- metric hues ---------------------------------------------------
    readonly property color neon:      dark ? "#2BB8FF" : "#1488D6"
    readonly property color neonCyan:  dark ? "#2BD8FF" : "#0E9DC4"
    readonly property color ringSteps: dark ? "#2BD8FF" : "#0E9DC4"
    readonly property color ringHr:    dark ? "#FF6B6B" : "#E5484D"
    readonly property color ringCal:   dark ? "#FF9A4A" : "#E07B1F"
    readonly property color purple:    dark ? "#7A5CFF" : "#6A40E0"
    readonly property color mint:      dark ? "#4AD6A0" : "#1FA877"

    readonly property color accent: {
        if (accentName === "violet") return dark ? "#8C6BFF" : "#6A40E0";
        if (accentName === "coral")  return dark ? "#FF6B6B" : "#E5484D";
        if (accentName === "mint")   return dark ? "#4AD6A0" : "#1FA877";
        if (accentName === "pink")   return dark ? "#FF63C0" : "#D6248C";
        return dark ? "#2BB8FF" : "#1488D6";
    }

    readonly property var accents: [
        { key: "blue",   label: "Neon Blue", color: dark ? "#2BB8FF" : "#1488D6" },
        { key: "violet", label: "Violet",    color: dark ? "#8C6BFF" : "#6A40E0" },
        { key: "coral",  label: "Coral",     color: dark ? "#FF6B6B" : "#E5484D" },
        { key: "mint",   label: "Mint",      color: dark ? "#4AD6A0" : "#1FA877" },
        { key: "pink",   label: "Pink",      color: dark ? "#FF63C0" : "#D6248C" }
    ]

    // ---- rhythm --------------------------------------------------------
    // Everything lands on the 8px grid unit; xs is the only half step.
    readonly property real xs: units.gu(0.5)
    readonly property real s:  units.gu(1)
    readonly property real m:  units.gu(1.5)
    readonly property real l:  units.gu(2)
    readonly property real xl: units.gu(3)
    readonly property real xxl: units.gu(4.5)

    readonly property real radiusCard: units.gu(2.5)
    readonly property real radiusTile: units.gu(2.25)
    readonly property real radiusPill: units.gu(1.25)

    // ---- type ----------------------------------------------------------
    readonly property string face: "Ubuntu"
    readonly property int micro: units.dp(10)
    readonly property int caption: units.dp(12)
    readonly property int body: units.dp(14)
    readonly property int subtitle: units.dp(16)
    readonly property int title: units.dp(21)
    readonly property int headline: units.dp(28)
    readonly property int display: units.dp(40)
    readonly property int hero: units.dp(54)

    // ---- motion ----------------------------------------------------------
    readonly property int fast: 140
    readonly property int med: 260
    readonly property int slow: 420
    readonly property int ring: 820

    // ---- helpers ---------------------------------------------------------
    // Guards every factor that reaches a gradient stop or a bar width: a NaN
    // there is not a visual glitch, it takes the scene graph down.
    function clamp01(v) {
        if (v === undefined || v === null || isNaN(v)) return 0;
        if (v < 0) return 0;
        if (v > 1) return 1;
        return v;
    }
    function safe(v) {
        return (v === undefined || v === null || isNaN(v) || !isFinite(v)) ? 0 : v;
    }

    function shade(c, f) {
        return Qt.rgba(c.r * f, c.g * f, c.b * f, c.a);
    }
    function tint(c, f) {
        return Qt.rgba(c.r + (1 - c.r) * f, c.g + (1 - c.g) * f, c.b + (1 - c.b) * f, c.a);
    }
    function alpha(c, a) {
        return Qt.rgba(c.r, c.g, c.b, a);
    }

    // Metric catalogue shared by Today / Health / Fitness. `key` matches the
    // metric keys used by GET /api/health.
    function metricColor(key) {
        switch (key) {
        case "steps": return ringSteps;
        case "distance": return accent;
        case "activetime":
        case "active_minutes": return mint;
        case "calories": return ringHr;
        case "intensity": return neonCyan;
        case "sleep": return purple;
        case "heart_rate":
        case "resting_hr": return ringHr;
        case "body_energy": return mint;
        case "stress": return ringCal;
        case "spo2": return neonCyan;
        case "hrv": return accent;
        case "respiration": return purple;
        }
        return accent;
    }

    function metricLabel(key) {
        switch (key) {
        case "steps": return "Steps";
        case "distance": return "Distance";
        case "activetime": return "Active time";
        case "calories": return "Calories";
        case "intensity": return "Intensity";
        case "sleep": return "Sleep";
        case "heart_rate": return "Heart rate";
        case "resting_hr": return "Resting HR";
        case "body_energy": return "Body Battery";
        case "stress": return "Stress";
        case "spo2": return "Blood oxygen";
        case "hrv": return "HRV";
        case "respiration": return "Respiration";
        }
        return key;
    }

    function metricGlyph(key) {
        switch (key) {
        case "steps": return "steps";
        case "distance": return "route";
        case "activetime":
        case "intensity": return "bolt";
        case "calories": return "flame";
        case "sleep": return "moon";
        case "heart_rate":
        case "resting_hr": return "pulse";
        case "body_energy": return "battery";
        case "stress": return "gauge";
        case "spo2": return "drop";
        case "hrv": return "heart";
        case "respiration": return "wave";
        }
        return "star";
    }

    // Lower is better for these, so a negative delta is the good direction.
    function metricInverted(key) {
        return key === "stress" || key === "resting_hr" || key === "respiration";
    }

    // Normalised 0..1 gauge factor per metric (docs §3.1 resolveMetric).
    function metricFactor(key, v) {
        if (v === undefined || v === null || v <= 0) return 0;
        switch (key) {
        case "heart_rate": return Math.max(0, Math.min(1, (v - 40) / 160));
        case "resting_hr": return Math.max(0, Math.min(1, (v - 30) / 70));
        case "body_energy": return Math.min(1, v / 100);
        case "stress": return Math.min(1, v / 100);
        case "spo2": return Math.min(1, v / 100);
        case "hrv": return Math.min(1, v / 120);
        case "respiration": return Math.max(0, Math.min(1, (v - 6) / 24));
        }
        return Math.min(1, v / 100);
    }

    // Sleep score bands (docs §2.4).
    function sleepQuality(score) {
        if (score >= 85) return "Excellent";
        if (score >= 70) return "Good";
        if (score >= 50) return "Fair";
        return "Poor";
    }
    function sleepColor(score) {
        if (score >= 85) return mint;
        if (score >= 70) return neonCyan;
        if (score >= 50) return ringCal;
        return ringHr;
    }
    function stageColor(stage) {
        switch (stage) {
        case "deep": return purple;
        case "light": return neonCyan;
        case "rem": return neon;
        case "awake": return ringHr;
        }
        return cardAlt;
    }
}
