import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt
import "../js/I18n.js" as I18n

Item {
    id: page

    signal openWorkout(var workout)

    readonly property var list: Store.workouts || []
    readonly property bool waiting: Store.workouts === null && (Store.workoutsLoading || !Store.everAnswered)

    readonly property var weekStats: {
        var since = Date.now() - 7 * 86400000;
        var count = 0, secs = 0;
        for (var i = 0; i < list.length; i++) {
            if (list[i].startMs < since) continue;
            count++;
            if (list[i].endMs > list[i].startMs) secs += (list[i].endMs - list[i].startMs) / 1000;
        }
        return { count: count, seconds: secs };
    }

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            kicker: I18n.t("fitness.kicker")
            title: I18n.t("fitness.title")
            trailingGlyph: "sync"
            onTrailing: Store.loadWorkouts()
        }

        Card {
            width: parent.width
            visible: !page.waiting && page.list.length > 0

            Row {
                width: parent.width
                spacing: Pulse.xl

                Column {
                    spacing: units.dp(2)
                    Label {
                        text: "" + page.weekStats.count
                        color: Pulse.accent
                        font.family: Pulse.face
                        font.pixelSize: Pulse.display
                        font.weight: Font.Light
                    }
                    Label {
                        text: I18n.isRu() ? "тренировок за неделю" : "sessions this week"
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.caption
                    }
                }

                Column {
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: units.dp(2)
                    Label {
                        text: page.weekStats.seconds > 0 ? Fmt.clock(page.weekStats.seconds) : "\u2013"
                        color: Pulse.text
                        font.family: Pulse.face
                        font.pixelSize: Pulse.title
                        font.weight: Font.DemiBold
                    }
                    Label {
                        text: I18n.isRu() ? "время в движении" : "moving time"
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.caption
                    }
                }
            }
        }

        Column {
            width: parent.width
            spacing: Pulse.m
            visible: page.waiting

            Repeater {
                model: 4
                delegate: Skeleton {
                    width: parent.width
                    height: units.gu(7)
                    radius: Pulse.radiusCard
                    delay: index * 120
                }
            }
        }

        SectionTitle {
            width: parent.width
            text: I18n.t("fitness.all_workouts")
            visible: !page.waiting && page.list.length > 0
        }

        Card {
            width: parent.width
            padding: page.list.length > 0 ? Pulse.s : Pulse.l
            spacing: 0
            visible: !page.waiting

            EmptyState {
                width: parent.width
                visible: page.list.length === 0
                glyph: "timer"
                title: Store.online ? I18n.t("fitness.no_workouts_title") : I18n.t("status.daemon_offline")
                hint: Store.online
                      ? I18n.t("fitness.no_workouts_hint")
                      : I18n.t("today.daemon_unreachable")
                action: Store.online && Store.connected ? I18n.t("action.sync_now") : ""
                onActionTriggered: Store.startSync()
            }

            Repeater {
                model: page.list
                delegate: WorkoutRow {
                    width: parent.width
                    workout: modelData
                    divider: index < page.list.length - 1
                    onClicked: page.openWorkout(modelData)
                }
            }
        }
    }
}
