import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

Rectangle {
    id: root

    property string text: ""
    property string glyph: ""
    // "primary" | "ghost" | "danger"
    property string kind: "ghost"
    property bool busy: false
    property bool enabledLook: true
    signal clicked()

    readonly property color base: kind === "primary" ? Pulse.accent
                                : kind === "danger" ? Pulse.alpha(Pulse.ringHr, 0.16)
                                : Pulse.cardAlt
    readonly property color ink: kind === "primary" ? Pulse.onAccent
                               : kind === "danger" ? Pulse.ringHr
                               : Pulse.text

    implicitHeight: units.gu(5)
    implicitWidth: row.width + Pulse.xl
    radius: Pulse.radiusPill
    color: base
    opacity: enabledLook ? 1 : 0.4

    Row {
        id: row
        anchors.centerIn: parent
        spacing: Pulse.s

        Glyph {
            visible: root.glyph.length > 0 && !root.busy
            name: root.glyph
            size: units.gu(2.25)
            color: root.ink
            anchors.verticalCenter: parent.verticalCenter
        }

        Item {
            visible: root.busy
            width: units.gu(2.25)
            height: units.gu(2.25)
            anchors.verticalCenter: parent.verticalCenter
            Glyph {
                anchors.fill: parent
                name: "sync"
                size: units.gu(2.25)
                color: root.ink
                RotationAnimator on rotation {
                    running: root.busy
                    loops: Animation.Infinite
                    from: 0; to: 360; duration: 1100
                }
            }
        }

        Label {
            text: root.text
            color: root.ink
            font.family: Pulse.face
            font.pixelSize: Pulse.body
            font.weight: Font.DemiBold
            anchors.verticalCenter: parent.verticalCenter
        }
    }

    Rectangle {
        anchors.fill: parent
        radius: parent.radius
        color: Pulse.text
        opacity: tap.pressed ? 0.10 : 0
        Behavior on opacity { NumberAnimation { duration: Pulse.fast } }
    }

    MouseArea {
        id: tap
        anchors.fill: parent
        enabled: root.enabledLook
        onClicked: root.clicked()
    }
}
