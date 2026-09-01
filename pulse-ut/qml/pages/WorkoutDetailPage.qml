import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Api.js" as Api
import "../js/Fmt.js" as Fmt

Item {
    id: page

    // The list entry is shown immediately; GET /api/workouts/{id} then fills
    // in the track without the screen ever being blank.
    property var stub: null
    property var detail: null
    property bool loading: false
    property string error: ""
    signal back()

    readonly property var w: detail ? detail : stub
    readonly property var track: detail && detail.track ? detail.track : []
    readonly property real seconds: w && w.endMs > w.startMs ? (w.endMs - w.startMs) / 1000 : 0

    // Runners read pace, cyclists read speed. Everything else follows the
    // user's unit preference.
    readonly property bool footSport: Fmt.isFootSport(w ? w.sport : -1)
    readonly property bool imperial: Store.settings.units === "imperial"
    readonly property real speedFactor: imperial ? 2.2369363 : 3.6
    readonly property string speedUnit: imperial ? "mph" : "km/h"

    function speedText(mps) {
        return Fmt.trimNum(mps * speedFactor, 1) + " " + speedUnit;
    }

    Component.onCompleted: load()

    function load() {
        if (!stub || stub.id === undefined) return;
        loading = true;
        error = "";
        Api.workout(stub.id, function (r) {
            detail = r;
            loading = false;
        }, function (msg) {
            error = msg;
            loading = false;
        });
    }

    function column(field) {
        var out = [];
        for (var i = 0; i < track.length; i++) {
            var v = track[i][field];
            out.push(v === undefined || v === null ? 0 : v);
        }
        return out;
    }

    function nonEmpty(field) {
        for (var i = 0; i < track.length; i++) {
            var v = track[i][field];
            if (v !== undefined && v !== null && v > 0) return true;
        }
        return false;
    }

    // The daemon's summary object is free-form; render whatever it sends with
    // sensible units instead of hiding unknown fields.
    readonly property var summaryEntries: {
        var out = [];
        var s = w && w.summary ? w.summary : null;
        if (!s) return out;
        for (var k in s) {
            var v = s[k];
            if (v === null || v === undefined || v === "") continue;
            if (typeof v === "object") continue;
            out.push({ label: humanise(k), value: render(k, v) });
        }
        return out;
    }

    function humanise(key) {
        var t = key.replace(/([a-z0-9])([A-Z])/g, "$1 $2").toLowerCase();
        t = t.replace(/ m$/, "").replace(/ sec$/, "").replace(/ ms$/, "").replace(/ kcal$/, "");
        t = t.replace(/^avg /, "avg ").replace(/^max /, "max ");
        return t.charAt(0).toUpperCase() + t.slice(1);
    }

    function render(key, v) {
        if (typeof v !== "number") return "" + v;
        if (/M$/.test(key) || /distance/i.test(key))
            return Fmt.distance(v, Store.settings.units);
        if (/Sec$/.test(key) || /duration/i.test(key) || /time/i.test(key))
            return Fmt.clock(v);
        if (/Ms$/.test(key))
            return Fmt.timeOfDay(v);
        if (/speed/i.test(key))
            return page.footSport ? Fmt.pace(v, Store.settings.units) : page.speedText(v);
        if (/calor/i.test(key))
            return Fmt.thousands(v) + " kcal";
        if (/heart|hr$/i.test(key))
            return Math.round(v) + " bpm";
        if (/cadence/i.test(key))
            return Math.round(v) + " spm";
        if (/power/i.test(key))
            return Math.round(v) + " W";
        return Fmt.trimNum(v, v % 1 === 0 ? 0 : 1);
    }

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            showBack: true
            kicker: page.w ? Fmt.dateShort(page.w.startMs) + " \u00b7 " + Fmt.timeOfDay(page.w.startMs) : ""
            title: Fmt.workoutTitle(page.w)
            onBack: page.back()
        }

        // ---- headline numbers -------------------------------------------
        Card {
            width: parent.width

            Row {
                width: parent.width
                spacing: Pulse.xl

                Column {
                    spacing: units.dp(2)
                    Label {
                        text: Fmt.clock(page.seconds)
                        color: Pulse.accent
                        font.family: Pulse.face
                        font.pixelSize: Pulse.display
                        font.weight: Font.Light
                    }
                    Label {
                        text: "duration"
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.caption
                    }
                }

                Column {
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: units.dp(2)
                    Label {
                        text: Fmt.timeOfDay(page.w ? page.w.startMs : 0) + " \u2192 " +
                              Fmt.timeOfDay(page.w ? page.w.endMs : 0)
                        color: Pulse.text
                        font.family: Pulse.face
                        font.pixelSize: Pulse.subtitle
                        font.weight: Font.DemiBold
                    }
                    Label {
                        text: Fmt.sportName(page.w ? page.w.sport : -1)
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.caption
                    }
                }
            }
        }

        // ---- route ---------------------------------------------------------
        SectionTitle {
            width: parent.width
            text: "Route"
            visible: routeMap.points > 1
        }

        Card {
            width: parent.width
            visible: routeMap.points > 1

            RouteMap {
                id: routeMap
                width: parent.width
                height: units.gu(24)
                track: page.track
                hue: Pulse.accent
            }
        }

        // ---- summary --------------------------------------------------------
        SectionTitle {
            width: parent.width
            text: "Summary"
            visible: page.summaryEntries.length > 0
        }

        Card {
            width: parent.width
            visible: page.summaryEntries.length > 0

            Grid {
                id: sumGrid
                width: parent.width
                columns: 2
                rowSpacing: Pulse.l
                columnSpacing: Pulse.l

                Repeater {
                    model: page.summaryEntries
                    delegate: Column {
                        width: (sumGrid.width - sumGrid.columnSpacing) / 2
                        spacing: units.dp(2)
                        Label {
                            width: parent.width
                            text: modelData.label.toUpperCase()
                            color: Pulse.textDim
                            font.family: Pulse.face
                            font.pixelSize: Pulse.micro
                            font.letterSpacing: units.dp(1.2)
                            elide: Text.ElideRight
                        }
                        Label {
                            width: parent.width
                            text: modelData.value
                            color: Pulse.text
                            font.family: Pulse.face
                            font.pixelSize: Pulse.subtitle
                            font.weight: Font.DemiBold
                            elide: Text.ElideRight
                        }
                    }
                }
            }
        }

        // ---- traces ------------------------------------------------------
        SectionTitle {
            width: parent.width
            text: "Traces"
            visible: page.track.length > 1
        }

        Column {
            width: parent.width
            spacing: Pulse.m
            visible: page.track.length > 1

            Repeater {
                model: [
                    { field: "heartRate", label: "Heart rate", unit: "bpm", hue: Pulse.ringHr, dec: 0 },
                    page.footSport
                        ? { field: "speed", label: "Pace", unit: "min/" + Fmt.distanceUnit(Store.settings.units),
                            hue: Pulse.accent, dec: 1 }
                        : { field: "speed", label: "Speed", unit: page.speedUnit, hue: Pulse.accent, dec: 1 },
                    { field: "altitude", label: "Altitude", unit: "m", hue: Pulse.mint, dec: 0 },
                    { field: "cadence", label: "Cadence", unit: "spm", hue: Pulse.purple, dec: 0 },
                    { field: "power", label: "Power", unit: "W", hue: Pulse.ringCal, dec: 0 }
                ]

                delegate: Card {
                    width: parent.width
                    visible: page.nonEmpty(modelData.field)

                    Row {
                        spacing: Pulse.xs
                        Rectangle {
                            width: units.dp(8); height: units.dp(8); radius: width / 2
                            color: modelData.hue
                            anchors.verticalCenter: parent.verticalCenter
                        }
                        Label {
                            text: modelData.label + " \u00b7 " + modelData.unit
                            color: Pulse.textDim
                            font.family: Pulse.face
                            font.pixelSize: Pulse.caption
                            anchors.verticalCenter: parent.verticalCenter
                        }
                    }

                    SeriesChart {
                        width: parent.width
                        height: units.gu(13)
                        hue: modelData.hue
                        decimals: modelData.dec
                        zeroIsGap: true
                        values: {
                            var raw = page.column(modelData.field);
                            if (modelData.field !== "speed") return raw;
                            var out = [];
                            for (var i = 0; i < raw.length; i++) {
                                out.push(page.footSport
                                         ? Fmt.paceMinutes(raw[i], Store.settings.units)
                                         : raw[i] * page.speedFactor);
                            }
                            return out;
                        }
                    }
                }
            }
        }

        // ---- states ---------------------------------------------------------
        Card {
            width: parent.width
            visible: page.loading || (page.error.length > 0) ||
                     (!page.loading && page.error.length === 0 && page.track.length === 0)

            Skeleton {
                width: parent.width
                height: units.gu(10)
                visible: page.loading
            }

            EmptyState {
                width: parent.width
                visible: page.error.length > 0
                glyph: "close"
                title: "Could not load this workout"
                hint: page.error
                action: "Retry"
                onActionTriggered: page.load()
            }

            EmptyState {
                width: parent.width
                visible: !page.loading && page.error.length === 0 && page.track.length === 0
                glyph: "route"
                title: "No detailed trace"
                hint: "This activity was stored as a summary only \u2014 the watch did not send per-second samples."
            }
        }
    }
}
