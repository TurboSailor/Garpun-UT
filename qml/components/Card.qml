import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Base surface: a padded vertical stack. Every panel in the app is this
// rectangle; nothing nests cards inside cards, depth is expressed with the
// alt surface tone instead.
Rectangle {
    id: root

    property real padding: Pulse.l
    property real spacing: Pulse.m
    property bool interactive: false
    property bool alt: false
    property alias contentHeight: content.height
    signal clicked()

    default property alias contentData: content.data

    color: alt ? Pulse.cardAlt : Pulse.card
    radius: Pulse.radiusCard
    implicitHeight: content.height + 2 * padding

    Column {
        id: content
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: root.padding
        spacing: root.spacing
    }

    Rectangle {
        anchors.fill: parent
        radius: parent.radius
        color: Pulse.text
        opacity: press.pressed ? 0.06 : 0
        visible: root.interactive
        Behavior on opacity { NumberAnimation { duration: Pulse.fast } }
    }

    MouseArea {
        id: press
        anchors.fill: parent
        enabled: root.interactive
        onClicked: root.clicked()
    }
}
