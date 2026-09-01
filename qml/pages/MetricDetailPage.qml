import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt
import "../js/I18n.js" as I18n

Item {
    id: page

    property var metric: null
    signal back()

    readonly property string key: metric && metric.key ? metric.key : ""
    readonly property color hue: Pulse.metricColor(key)
    readonly property int decimals: key === "respiration" || key === "hrv" ? 1 : 0
    readonly property var series: metric && metric.series ? metric.series : []
    readonly property var values: {
        var out = [];
        for (var i = 0; i < series.length; i++) out.push(series[i].value);
        return out;
    }
    readonly property var stats: {
        var mn = Infinity, mx = -Infinity, s = 0, n = 0;
        for (var i = 0; i < values.length; i++) {
            var v = values[i];
            if (!(v > 0)) continue;
            if (v < mn) mn = v;
            if (v > mx) mx = v;
            s += v; n++;
        }
        return { min: n ? mn : 0, max: n ? mx : 0, avg: n ? s / n : 0, count: n };
    }

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            showBack: true
            kicker: I18n.t("metric_detail.last_days", [Store.healthDays])
            title: page.key.length ? Pulse.metricLabel(page.key) : (page.metric ? page.metric.label : "")
            onBack: page.back()
        }

        Card {
            width: parent.width

            Row {
                spacing: units.dp(4)
                Label {
                    id: big
                    text: page.metric && page.metric.latest > 0
                          ? Fmt.trimNum(page.metric.latest, page.decimals) : "\u2013"
                    color: page.hue
                    font.family: Pulse.face
                    font.pixelSize: Pulse.hero
                    font.weight: Font.Light
                }
                Label {
                    anchors.baseline: big.baseline
                    text: page.key.length ? Pulse.metricUnit(page.key) : ""
                    color: Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.subtitle
                }
            }

            SeriesChart {
                width: parent.width
                height: units.gu(16)
                values: page.values
                hue: page.hue
                decimals: page.decimals
                zeroIsGap: true
                visible: page.stats.count > 1
            }

            EmptyState {
                width: parent.width
                visible: page.stats.count <= 1
                glyph: Pulse.metricGlyph(page.key)
                title: I18n.t("metric_detail.not_enough_history")
                hint: I18n.t("metric_detail.not_enough_hint")
            }

            Row {
                width: parent.width
                visible: page.stats.count > 0
                spacing: Pulse.xl

                Repeater {
                    model: [
                        { label: I18n.t("metric_detail.min"), value: page.stats.min },
                        { label: I18n.t("metric_detail.avg"), value: page.stats.avg },
                        { label: I18n.t("metric_detail.max"), value: page.stats.max }
                    ]
                    delegate: Column {
                        spacing: units.dp(2)
                        Label {
                            text: modelData.label.toUpperCase()
                            color: Pulse.textDim
                            font.family: Pulse.face
                            font.pixelSize: Pulse.micro
                            font.letterSpacing: units.dp(1.2)
                        }
                        Label {
                            text: Fmt.trimNum(modelData.value, page.decimals)
                            color: Pulse.text
                            font.family: Pulse.face
                            font.pixelSize: Pulse.subtitle
                            font.weight: Font.DemiBold
                        }
                    }
                }
            }
        }

        SectionTitle { width: parent.width; text: I18n.t("metric_detail.samples"); glyph: "calendar" }

        Card {
            width: parent.width
            padding: Pulse.s
            spacing: 0

            Repeater {
                model: page.series.slice().reverse()
                delegate: Item {
                    width: parent.width
                    height: units.gu(5)

                    Label {
                        anchors.left: parent.left
                        anchors.leftMargin: Pulse.m
                        anchors.verticalCenter: parent.verticalCenter
                        text: Fmt.dateShort(modelData.tsMs) + " \u00b7 " + Fmt.timeOfDay(modelData.tsMs)
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.caption
                    }

                    Label {
                        anchors.right: parent.right
                        anchors.rightMargin: Pulse.m
                        anchors.verticalCenter: parent.verticalCenter
                        text: modelData.value > 0
                              ? Fmt.trimNum(modelData.value, page.decimals) + " " + Pulse.metricUnit(page.key)
                              : "\u2013"
                        color: Pulse.text
                        font.family: Pulse.face
                        font.pixelSize: Pulse.body
                        font.weight: Font.DemiBold
                    }

                    Rectangle {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.margins: Pulse.m
                        anchors.bottom: parent.bottom
                        height: units.dp(1)
                        color: Pulse.hairline
                        visible: index < page.series.length - 1
                    }
                }
            }

            Label {
                visible: page.series.length === 0
                width: parent.width
                text: I18n.t("metric_detail.no_samples")
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.body
            }
        }
    }
}
