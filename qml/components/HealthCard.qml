import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/Fmt.js" as Fmt
import "../js/I18n.js" as I18n

// One metric from GET /api/health. `headline` gives it the full width and the
// large type; the compact form is used for the grid below.
Rectangle {
    id: root

    property var metric: null
    property bool headline: false
    property bool loading: false
    signal clicked()

    readonly property string key: metric && metric.key ? metric.key : ""
    readonly property color hue: Pulse.metricColor(key)
    readonly property var series: {
        var out = [];
        if (!metric || !metric.series) return out;
        for (var i = 0; i < metric.series.length; i++) out.push(metric.series[i].value);
        return out;
    }
    readonly property real average: {
        var n = 0, s = 0;
        for (var i = 0; i < series.length - 1; i++) {
            if (series[i] > 0) { s += series[i]; n++; }
        }
        return n > 0 ? s / n : 0;
    }
    readonly property int decimals: key === "respiration" || key === "hrv" ? 1 : 0

    radius: Pulse.radiusCard
    color: Pulse.card
    // Floors keep the metric grid on a level baseline regardless of which
    // cards happen to have a delta to show.
    implicitHeight: headline
                    ? Math.max(units.gu(20), head.height + spark.height + 3 * Pulse.l)
                    : Math.max(units.gu(15), compact.height + spark.height + 3 * Pulse.m)

    // ---- headline layout ------------------------------------------------
    Item {
        id: head
        visible: root.headline
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: Pulse.l
        height: visible ? Math.max(chip.height, texts.height) : 0

        Rectangle {
            id: chip
            width: units.gu(4.5)
            height: width
            radius: Pulse.radiusTile
            color: Pulse.alpha(root.hue, 0.16)
            Glyph {
                anchors.centerIn: parent
                name: Pulse.metricGlyph(root.key)
                size: units.gu(2.5)
                color: root.hue
            }
        }

        Column {
            id: texts
            anchors.left: chip.right
            anchors.leftMargin: Pulse.m
            anchors.right: parent.right
            spacing: units.dp(2)

            Label {
                text: root.key.length ? Pulse.metricLabel(root.key) : (root.metric ? root.metric.label : "")
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }

            Row {
                spacing: units.dp(4)
                Label {
                    id: bigValue
                    text: root.metric && root.metric.latest > 0
                          ? Fmt.trimNum(root.metric.latest, root.decimals) : "\u2013"
                    color: root.hue
                    font.family: Pulse.face
                    font.pixelSize: Pulse.display
                    font.weight: Font.DemiBold
                }
                Label {
                    anchors.baseline: bigValue.baseline
                    text: root.key.length ? Pulse.metricUnit(root.key) : ""
                    color: Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.body
                }
            }

            DeltaChip {
                visible: root.metric && root.metric.latest > 0
                delta: root.metric ? root.metric.delta : 0
                reference: root.average
                inverted: Pulse.metricInverted(root.key)
                decimals: root.decimals
                unit: root.key.length ? Pulse.metricUnit(root.key) : ""
            }
        }
    }

    // ---- compact layout ---------------------------------------------------
    Column {
        id: compact
        visible: !root.headline
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: Pulse.m
        spacing: units.dp(2)

        Row {
            spacing: Pulse.xs
            Glyph {
                name: Pulse.metricGlyph(root.key)
                size: units.gu(2)
                color: root.hue
                anchors.verticalCenter: parent.verticalCenter
            }
            Label {
                text: root.key.length ? Pulse.metricLabel(root.key) : (root.metric ? root.metric.label : "")
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
                anchors.verticalCenter: parent.verticalCenter
            }
        }

        Row {
            spacing: units.dp(3)
            Label {
                id: smallValue
                text: root.metric && root.metric.latest > 0
                      ? Fmt.trimNum(root.metric.latest, root.decimals) : "\u2013"
                color: Pulse.text
                font.family: Pulse.face
                font.pixelSize: Pulse.title
                font.weight: Font.DemiBold
            }
            Label {
                anchors.baseline: smallValue.baseline
                text: root.key.length ? Pulse.metricUnit(root.key) : ""
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.micro
            }
        }

        DeltaChip {
            visible: root.metric && root.metric.latest > 0
            delta: root.metric ? root.metric.delta : 0
            reference: root.average
            inverted: Pulse.metricInverted(root.key)
            decimals: root.decimals
        }
    }

    Sparkline {
        id: spark
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.leftMargin: root.headline ? Pulse.l : Pulse.m
        anchors.rightMargin: root.headline ? Pulse.l : Pulse.m
        anchors.bottomMargin: root.headline ? Pulse.l : Pulse.m
        height: root.headline ? units.gu(7) : units.gu(3.5)
        values: root.series
        hue: root.hue
        visible: root.series.length > 1
    }

    // A metric with no history still needs to occupy its slot honestly.
    Label {
        visible: root.series.length <= 1
        anchors.left: spark.left
        anchors.bottom: spark.bottom
        text: I18n.t("metric_detail.no_samples")
        color: Pulse.textDim
        font.family: Pulse.face
        font.pixelSize: Pulse.micro
    }

    Rectangle {
        anchors.fill: parent
        radius: parent.radius
        color: Pulse.text
        opacity: tap.pressed ? 0.05 : 0
        Behavior on opacity { NumberAnimation { duration: Pulse.fast } }
    }

    MouseArea {
        id: tap
        anchors.fill: parent
        onClicked: root.clicked()
    }
}
