import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

Rectangle {
    id: root

    property real factor: 0
    property color hue: Pulse.accent

    height: units.dp(4)
    radius: height / 2
    color: Pulse.cardAlt

    Rectangle {
        height: parent.height
        radius: parent.radius
        color: root.hue
        width: Pulse.clamp01(root.factor) > 0
               ? Math.max(parent.height, Pulse.clamp01(root.factor) * parent.width)
               : 0
        Behavior on width { NumberAnimation { duration: Pulse.slow; easing.type: Easing.OutQuart } }
    }
}
