import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt

Item {
    id: page

    signal openMetric(var metric)

    readonly property var metrics: Store.health || []
    readonly property bool waiting: Store.health === null && (Store.healthLoading || !Store.everAnswered)

    // Body Battery leads when the daemon reports it; otherwise the first
    // metric it sent becomes the headline.
    readonly property int headlineIndex: {
        for (var i = 0; i < metrics.length; i++)
            if (metrics[i].key === "body_energy") return i;
        return metrics.length > 0 ? 0 : -1;
    }
    readonly property var rest: {
        var out = [];
        for (var i = 0; i < metrics.length; i++)
            if (i !== headlineIndex) out.push(metrics[i]);
        return out;
    }

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            kicker: "Trends"
            title: "Health"
            trailingGlyph: "sync"
            onTrailing: Store.loadHealth()
        }

        Segmented {
            width: parent.width
            options: [
                { key: "7", label: "7 days" },
                { key: "14", label: "14 days" },
                { key: "30", label: "30 days" }
            ]
            current: "" + Store.healthDays
            onPicked: {
                Store.healthDays = parseInt(key, 10);
                Store.loadHealth();
            }
        }

        // ---- loading ------------------------------------------------------
        Column {
            width: parent.width
            visible: page.waiting
            spacing: Pulse.m

            Skeleton { width: parent.width; height: units.gu(16); radius: Pulse.radiusCard }

            Grid {
                columns: 2
                spacing: Pulse.m
                Repeater {
                    model: 4
                    delegate: Skeleton {
                        width: (page.width - 2 * Pulse.l - Pulse.m) / 2
                        height: units.gu(12)
                        radius: Pulse.radiusCard
                        delay: index * 120
                    }
                }
            }
        }

        // ---- empty --------------------------------------------------------
        Card {
            width: parent.width
            visible: !page.waiting && page.metrics.length === 0

            EmptyState {
                width: parent.width
                glyph: "pulse"
                title: Store.online ? "No health history yet" : "Pulse daemon offline"
                hint: Store.online
                      ? "Wear the watch through a day and sync — heart rate, Body Battery, stress, SpO\u2082, HRV and respiration land here."
                      : "Start pulsed and this page fills in on the next sync."
                action: Store.online && Store.connected ? "Sync now" : ""
                onActionTriggered: Store.startSync()
            }
        }

        // ---- headline -------------------------------------------------------
        HealthCard {
            width: parent.width
            visible: !page.waiting && page.headlineIndex >= 0
            headline: true
            metric: page.headlineIndex >= 0 ? page.metrics[page.headlineIndex] : null
            onClicked: page.openMetric(metric)
        }

        // ---- grid -------------------------------------------------------
        Grid {
            id: grid
            width: parent.width
            columns: 2
            spacing: Pulse.m
            visible: !page.waiting && page.rest.length > 0

            readonly property real cell: (width - spacing) / 2

            Repeater {
                model: page.rest
                delegate: HealthCard {
                    width: grid.cell
                    metric: modelData
                    onClicked: page.openMetric(modelData)
                }
            }
        }

        Label {
            width: parent.width
            visible: !page.waiting && page.metrics.length > 0
            text: "Deltas compare the latest value with the average of the preceding days."
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.micro
            wrapMode: Text.WordWrap
        }
    }
}
