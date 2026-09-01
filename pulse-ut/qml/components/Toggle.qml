import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

Item {
    id: root

    property bool checked: false
    signal toggled(bool value)

    width: units.gu(5.5)
    height: units.gu(3)

    Rectangle {
        anchors.fill: parent
        radius: height / 2
        color: root.checked ? Pulse.accent : Pulse.cardAlt
        Behavior on color { ColorAnimation { duration: Pulse.med } }
    }

    Rectangle {
        width: parent.height - units.dp(6)
        height: width
        radius: width / 2
        y: units.dp(3)
        x: root.checked ? parent.width - width - units.dp(3) : units.dp(3)
        color: root.checked ? Pulse.onAccent : Pulse.textDim
        Behavior on x { NumberAnimation { duration: Pulse.med; easing.type: Easing.OutQuart } }
        Behavior on color { ColorAnimation { duration: Pulse.med } }
    }

    MouseArea {
        anchors.fill: parent
        anchors.margins: -Pulse.s
        onClicked: root.toggled(!root.checked)
    }
}
