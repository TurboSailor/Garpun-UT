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
        width: Math.max(root.factor > 0 ? parent.height : 0,
                        Math.min(1, Math.max(0, root.factor)) * parent.width)
        Behavior on width { NumberAnimation { duration: Pulse.slow; easing.type: Easing.OutQuart } }
    }
}
