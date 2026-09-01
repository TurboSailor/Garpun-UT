import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

Rectangle {
    id: root

    property string message: ""

    function show(text) {
        message = text;
        life.restart();
        opacity = 1;
    }

    width: Math.min(parent ? parent.width - 2 * Pulse.l : 0, label.implicitWidth + 2 * Pulse.l)
    height: label.implicitHeight + 2 * Pulse.m
    radius: Pulse.radiusPill
    color: Pulse.cardAlt
    opacity: 0
    visible: opacity > 0

    Behavior on opacity { NumberAnimation { duration: Pulse.med } }

    Label {
        id: label
        anchors.centerIn: parent
        width: root.width - 2 * Pulse.l
        text: root.message
        color: Pulse.text
        font.family: Pulse.face
        font.pixelSize: Pulse.body
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.WordWrap
    }

    Timer {
        id: life
        interval: 2600
        onTriggered: root.opacity = 0
    }
}
