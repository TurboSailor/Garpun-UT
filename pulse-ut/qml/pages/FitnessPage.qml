import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt

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
            kicker: "Activities"
            title: "Fitness"
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
                        text: "sessions this week"
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
                        text: "moving time"
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
            text: "All workouts"
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
                title: Store.online ? "No workouts recorded" : "Pulse daemon offline"
                hint: Store.online
                      ? "Start an activity on the watch. After the next sync it shows up here with its full trace."
                      : "Pulse cannot reach pulsed on 127.0.0.1:21830."
                action: Store.online && Store.connected ? "Sync now" : ""
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
