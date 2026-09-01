import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Placeholder block used while a request is in flight. It breathes rather
// than shimmers: one animated property, no gradient sweep.
Rectangle {
    id: root

    property int delay: 0

    height: units.gu(2)
    radius: units.dp(6)
    color: Pulse.cardAlt

    SequentialAnimation on opacity {
        running: root.visible
        loops: Animation.Infinite
        PauseAnimation { duration: root.delay }
        NumberAnimation { to: 0.4; duration: 620; easing.type: Easing.InOutQuad }
        NumberAnimation { to: 0.9; duration: 620; easing.type: Easing.InOutQuad }
    }
}
