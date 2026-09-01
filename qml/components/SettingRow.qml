import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// One settings line. `control` slots in a Toggle, a value label, a stepper —
// whatever the setting needs — and stays right aligned.
Item {
    id: root

    property string title: ""
    property string subtitle: ""
    property string glyph: ""
    property bool divider: true
    property bool clickable: false
    signal clicked()

    default property alias controlData: control.data

    width: parent ? parent.width : 0
    implicitHeight: Math.max(texts.height, control.height) + 2 * Pulse.m

    Rectangle {
        anchors.fill: parent
        color: Pulse.text
        opacity: tap.pressed && root.clickable ? 0.05 : 0
        Behavior on opacity { NumberAnimation { duration: Pulse.fast } }
    }

    Glyph {
        id: icon
        visible: root.glyph.length > 0
        name: root.glyph
        size: units.gu(2.25)
        color: Pulse.textDim
        anchors.left: parent.left
        anchors.verticalCenter: parent.verticalCenter
    }

    Column {
        id: texts
        anchors.left: icon.visible ? icon.right : parent.left
        anchors.leftMargin: icon.visible ? Pulse.m : 0
        anchors.right: control.left
        anchors.rightMargin: Pulse.m
        anchors.verticalCenter: parent.verticalCenter
        spacing: units.dp(2)

        Label {
            width: parent.width
            text: root.title
            color: Pulse.text
            font.family: Pulse.face
            font.pixelSize: Pulse.body
            elide: Text.ElideRight
        }
        Label {
            width: parent.width
            visible: root.subtitle.length > 0
            text: root.subtitle
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.caption
            wrapMode: Text.WordWrap
        }
    }

    Item {
        id: control
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
        width: childrenRect.width
        height: childrenRect.height
    }

    Rectangle {
        visible: root.divider
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: units.dp(1)
        color: Pulse.hairline
    }

    MouseArea {
        id: tap
        anchors.fill: parent
        enabled: root.clickable
        onClicked: root.clicked()
    }
}
