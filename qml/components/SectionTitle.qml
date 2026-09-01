import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

Item {
    id: root

    property string text: ""
    property string action: ""
    property string glyph: ""
    signal actionTriggered()

    height: Math.max(label.height, link.height, units.gu(2))

    Glyph {
        id: mark
        anchors.left: parent.left
        anchors.verticalCenter: parent.verticalCenter
        visible: root.glyph.length > 0
        name: root.glyph
        size: units.gu(1.75)
        weight: 2.2
        color: Pulse.textDim
    }

    Label {
        id: label
        anchors.left: mark.visible ? mark.right : parent.left
        anchors.leftMargin: mark.visible ? Pulse.xs : 0
        anchors.verticalCenter: parent.verticalCenter
        text: root.text.toUpperCase()
        color: Pulse.textDim
        font.family: Pulse.face
        font.pixelSize: Pulse.micro
        font.weight: Font.DemiBold
        font.letterSpacing: units.dp(1.4)
    }

    Label {
        id: link
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
        visible: root.action.length > 0
        text: root.action
        color: Pulse.accent
        font.family: Pulse.face
        font.pixelSize: Pulse.caption

        MouseArea {
            anchors.fill: parent
            anchors.margins: -Pulse.s
            onClicked: root.actionTriggered()
        }
    }
}
