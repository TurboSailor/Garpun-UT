import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Seven-night pills. An empty night is a dot at 25% alpha rather than a gap,
// so the week keeps its shape.
Item {
    id: root

    property var points: []        // [{label, value, tint}]
    property color hue: Pulse.purple
    property real maxHeight: units.gu(11)
    property string suffix: ""

    readonly property real peak: {
        var m = 0;
        for (var i = 0; i < (points || []).length; i++)
            if (points[i].value > m) m = points[i].value;
        return m;
    }

    implicitHeight: maxHeight + Pulse.m + units.gu(2)

    Row {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        spacing: (points && points.length > 1)
                 ? Math.max(units.dp(6), (width - points.length * units.gu(4)) / (points.length - 1))
                 : 0

        Repeater {
            model: root.points || []

            delegate: Column {
                spacing: Pulse.s
                width: units.gu(4)

                Item {
                    width: parent.width
                    height: root.maxHeight

                    Rectangle {
                        anchors.horizontalCenter: parent.horizontalCenter
                        anchors.bottom: parent.bottom
                        width: units.gu(3)
                        radius: width / 2
                        color: modelData.tint !== undefined ? modelData.tint : root.hue
                        opacity: modelData.value > 0 ? 1 : 0.25
                        height: modelData.value > 0 && root.peak > 0
                                ? Math.max(units.gu(3), root.maxHeight * modelData.value / root.peak)
                                : units.gu(3)
                        Behavior on height { NumberAnimation { duration: Pulse.slow; easing.type: Easing.OutQuart } }
                    }
                }

                Label {
                    width: parent.width
                    horizontalAlignment: Text.AlignHCenter
                    text: modelData.label
                    color: Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.micro
                }
            }
        }
    }
}
