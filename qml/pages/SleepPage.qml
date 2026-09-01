import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt
import "../js/I18n.js" as I18n

Item {
    id: page

    readonly property var s: Store.sleep
    readonly property bool waiting: Store.sleep === null && (Store.sleepLoading || !Store.everAnswered)
    readonly property var totals: s && s.totals ? s.totals : null
    readonly property real asleep: totals ? (totals.deep || 0) + (totals.light || 0) + (totals.rem || 0) : 0
    readonly property real inBed: asleep + (totals ? (totals.awake || 0) : 0)
    readonly property int score: s && s.score > 0 ? s.score : 0
    readonly property bool hasNight: asleep > 0 || score > 0
    readonly property real goalMinutes: Store.settings.sleepGoalMinutes > 0 ? Store.settings.sleepGoalMinutes : 480

    property real shownScore: 0

    onScoreChanged: countUp.restart()
    Component.onCompleted: countUp.restart()

    // 900ms count-up after a 150ms beat, as in PulseSleepActivity.
    SequentialAnimation {
        id: countUp
        PauseAnimation { duration: 150 }
        NumberAnimation {
            target: page
            property: "shownScore"
            from: 0
            to: page.score
            duration: 900
            easing.type: Easing.OutCubic
        }
    }

    readonly property var trendPoints: {
        var out = [];
        if (!s || !s.trend) return out;
        for (var i = 0; i < s.trend.length; i++) {
            var d = s.trend[i];
            out.push({
                label: Fmt.DAYS_SHORT[Fmt.parseIso(d.date).getDay()],
                value: d.minutes || 0,
                tint: d.score > 0 ? Pulse.sleepColor(d.score) : Pulse.purple
            });
        }
        return out;
    }

    readonly property string insight: {
        if (!hasNight) return "";
        if (asleep >= goalMinutes)
            return (I18n.isRu() ? "Цель достигнута — " : "Goal met \u2014 ") + Fmt.durationTrim(asleep) +
                   (I18n.isRu() ? " сна при цели " : " asleep against a ") + Fmt.durationTrim(goalMinutes) + ".";
        if (asleep > 0 && asleep < goalMinutes)
            return (I18n.isRu() ? "Не хватает " : "") + Fmt.durationTrim(goalMinutes - asleep) +
                   (I18n.isRu() ? " до целевых " : " short of your ") + Fmt.durationTrim(goalMinutes) + ".";
        return "";
    }

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            kicker: Fmt.prettyDate(Store.date)
            title: I18n.t("sleep.title")
        }

        DateNav { width: parent.width }

        // ---- score hero -----------------------------------------------------
        Card {
            width: parent.width
            visible: !page.waiting

            EmptyState {
                width: parent.width
                visible: !page.hasNight
                glyph: "moon"
                title: Store.online ? I18n.t("sleep.no_night_title") : I18n.t("status.daemon_offline")
                hint: Store.online
                      ? I18n.t("sleep.no_night_hint")
                      : I18n.t("sleep.start_pulsed_hint")
            }

            Item {
                width: parent.width
                visible: page.hasNight
                height: Math.max(scoreCol.height, ringWrap.height)

                Column {
                    id: scoreCol
                    anchors.left: parent.left
                    anchors.right: ringWrap.left
                    anchors.rightMargin: Pulse.l
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: units.dp(2)

                    Row {
                        spacing: units.dp(6)
                        Label {
                            id: scoreLabel
                            text: page.score > 0 ? "" + Math.round(page.shownScore) : "\u2013"
                            color: Pulse.sleepColor(page.score)
                            font.family: Pulse.face
                            font.pixelSize: Pulse.hero
                            font.weight: Font.Light
                        }
                        Label {
                            anchors.baseline: scoreLabel.baseline
                            text: page.score > 0 ? Pulse.sleepQuality(page.score) : ""
                            color: Pulse.sleepColor(page.score)
                            font.family: Pulse.face
                            font.pixelSize: Pulse.subtitle
                            font.weight: Font.DemiBold
                        }
                    }

                    Label {
                        text: Fmt.durationTrim(page.asleep) + (I18n.isRu() ? " сна" : " asleep")
                        color: Pulse.text
                        font.family: Pulse.face
                        font.pixelSize: Pulse.body
                    }

                    Label {
                        text: page.s && page.s.startMs > 0
                              ? Fmt.timeOfDay(page.s.startMs) + " \u2192 " + Fmt.timeOfDay(page.s.endMs)
                              : ""
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.caption
                    }

                    Label {
                        visible: page.s && page.s.restlessMoments > 0
                        text: page.s ? page.s.restlessMoments + (I18n.isRu() ? " беспокойных движений" : " restless moments") : ""
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.caption
                    }
                }

                Item {
                    id: ringWrap
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    width: units.gu(12)
                    height: units.gu(12)

                    RingGauge {
                        anchors.fill: parent
                        thickness: units.dp(16)
                        progress: page.goalMinutes > 0 ? page.asleep / page.goalMinutes : 0
                        from: Pulse.purple
                        to: Pulse.neonCyan

                        Column {
                            anchors.centerIn: parent
                            spacing: 0
                            Label {
                                anchors.horizontalCenter: parent.horizontalCenter
                                text: page.goalMinutes > 0
                                      ? Math.round(100 * Math.min(1, page.asleep / page.goalMinutes)) + "%"
                                      : "\u2013"
                                color: Pulse.text
                                font.family: Pulse.face
                                font.pixelSize: Pulse.subtitle
                                font.weight: Font.DemiBold
                            }
                            Label {
                                anchors.horizontalCenter: parent.horizontalCenter
                                text: I18n.t("sleep.goal_share")
                                color: Pulse.textDim
                                font.family: Pulse.face
                                font.pixelSize: Pulse.micro
                            }
                        }
                    }
                }
            }

            Label {
                width: parent.width
                visible: page.insight.length > 0
                text: page.insight
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
                wrapMode: Text.WordWrap
            }
        }

        Skeleton {
            width: parent.width
            height: units.gu(20)
            radius: Pulse.radiusCard
            visible: page.waiting
        }

        // ---- stages ---------------------------------------------------------
        Card {
            width: parent.width
            visible: !page.waiting && page.hasNight

            StageBar {
                width: parent.width
                totals: page.totals
            }

            Row {
                width: parent.width
                spacing: Pulse.xl

                Repeater {
                    model: ["deep", "light", "rem", "awake"]
                    delegate: Column {
                        spacing: units.dp(1)
                        Label {
                            text: Fmt.durationTrim(page.totals ? (page.totals[modelData] || 0) : 0)
                            color: Pulse.text
                            font.family: Pulse.face
                            font.pixelSize: Pulse.body
                            font.weight: Font.DemiBold
                        }
                        Label {
                            text: page.inBed > 0
                                  ? Math.round(100 * (page.totals[modelData] || 0) / page.inBed) + "%"
                                  : "\u2013"
                            color: Pulse.stageColor(modelData)
                            font.family: Pulse.face
                            font.pixelSize: Pulse.micro
                        }
                    }
                }
            }
        }

        // ---- hypnogram --------------------------------------------------------
        SectionTitle {
            width: parent.width
            text: I18n.t("sleep.hypnogram")
            glyph: "moon"
            visible: !page.waiting && page.hasNight
        }

        Card {
            width: parent.width
            visible: !page.waiting && page.hasNight

            Hypnogram {
                width: parent.width
                stages: page.s && page.s.stages ? page.s.stages : []
                startMs: page.s ? page.s.startMs : 0
                endMs: page.s ? page.s.endMs : 0
            }

            Item {
                width: parent.width
                height: units.gu(2)

                Label {
                    anchors.left: parent.left
                    text: page.s ? Fmt.timeOfDay(page.s.startMs) : ""
                    color: Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.micro
                }
                Label {
                    anchors.right: parent.right
                    text: page.s ? Fmt.timeOfDay(page.s.endMs) : ""
                    color: Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.micro
                }
                Label {
                    visible: !page.s || !page.s.stages || page.s.stages.length === 0
                    anchors.centerIn: parent
                    text: I18n.t("sleep.no_stages")
                    color: Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.micro
                }
            }
        }

        // ---- trend ------------------------------------------------------------
        SectionTitle {
            width: parent.width
            text: I18n.t("sleep.last_7_nights")
            glyph: "trend_up"
            visible: !page.waiting && page.trendPoints.length > 0
        }

        Card {
            width: parent.width
            visible: !page.waiting && page.trendPoints.length > 0

            TrendBars {
                width: parent.width
                points: page.trendPoints
                hue: Pulse.purple
            }

            Label {
                width: parent.width
                text: {
                    var sum = 0, n = 0;
                    for (var i = 0; i < page.trendPoints.length; i++) {
                        if (page.trendPoints[i].value > 0) { sum += page.trendPoints[i].value; n++; }
                    }
                    if (n === 0) return I18n.t("sleep.no_nights_yet");
                    var nStr = I18n.isRu() ? (n + " ночей") : (n + (n === 1 ? " night" : " nights"));
                    return I18n.t("sleep.avg_nights", [Fmt.durationTrim(sum / n), nStr]);
                }
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }
        }

        // ---- naps ----------------------------------------------------------
        SectionTitle {
            width: parent.width
            text: I18n.t("sleep.naps")
            glyph: "clock"
            visible: !page.waiting && page.s && page.s.naps && page.s.naps.length > 0
        }

        Card {
            width: parent.width
            padding: Pulse.s
            spacing: 0
            visible: !page.waiting && page.s && page.s.naps && page.s.naps.length > 0

            Repeater {
                model: page.s && page.s.naps ? page.s.naps : []
                delegate: Item {
                    width: parent.width
                    height: units.gu(5)

                    Glyph {
                        id: napIcon
                        anchors.left: parent.left
                        anchors.leftMargin: Pulse.m
                        anchors.verticalCenter: parent.verticalCenter
                        name: "moon"
                        size: units.gu(2)
                        color: Pulse.purple
                    }
                    Label {
                        anchors.left: napIcon.right
                        anchors.leftMargin: Pulse.m
                        anchors.verticalCenter: parent.verticalCenter
                        text: Fmt.timeOfDay(modelData.startMs) + " \u2192 " + Fmt.timeOfDay(modelData.endMs)
                        color: Pulse.text
                        font.family: Pulse.face
                        font.pixelSize: Pulse.body
                    }
                    Label {
                        anchors.right: parent.right
                        anchors.rightMargin: Pulse.m
                        anchors.verticalCenter: parent.verticalCenter
                        text: Fmt.durationTrim(modelData.minutes)
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.body
                        font.weight: Font.DemiBold
                    }
                }
            }
        }
    }
}
