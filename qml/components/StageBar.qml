import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Proportional split of the night. Segments are separate pills so the ratio
// stays readable even when one stage is a sliver.
Item {
    id: root

    property var totals: null   // {deep, light, rem, awake} in minutes
    property bool legend: true

    readonly property var order: ["deep", "light", "rem", "awake"]
    readonly property real sum: {
        if (!totals) return 0;
        var s = 0;
        for (var i = 0; i < order.length; i++) s += Math.max(0, totals[order[i]] || 0);
        return s;
    }

    implicitHeight: bar.height + (legend ? legendRow.height + Pulse.m : 0)

    Row {
        id: bar
        width: parent.width
        height: units.dp(10)
        spacing: units.dp(3)

        Repeater {
            model: root.order
            delegate: Rectangle {
                readonly property real minutes: root.totals ? Math.max(0, root.totals[modelData] || 0) : 0
                visible: minutes > 0
                width: root.sum > 0
                       ? Math.max(units.dp(10), (root.width - units.dp(9)) * minutes / root.sum)
                       : 0
                height: parent.height
                radius: height / 2
                color: Pulse.stageColor(modelData)
                Behavior on width { NumberAnimation { duration: Pulse.slow; easing.type: Easing.OutQuart } }
            }
        }
    }

    // fallback track when there is nothing to show
    Rectangle {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        height: units.dp(10)
        radius: height / 2
        color: Pulse.cardAlt
        visible: root.sum <= 0
    }

    Row {
        id: legendRow
        visible: root.legend
        anchors.top: bar.bottom
        anchors.topMargin: Pulse.m
        spacing: Pulse.l

        Repeater {
            model: root.order
            delegate: Row {
                spacing: Pulse.xs
                Rectangle {
                    width: units.dp(8); height: units.dp(8); radius: width / 2
                    color: Pulse.stageColor(modelData)
                    anchors.verticalCenter: parent.verticalCenter
                }
                Label {
                    text: modelData.charAt(0).toUpperCase() + modelData.slice(1)
                    color: Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.micro
                    anchors.verticalCenter: parent.verticalCenter
                }
            }
        }
    }
}
