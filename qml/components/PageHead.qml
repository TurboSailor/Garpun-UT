import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Page header. Left aligned by design — the whole app reads down the left
// edge, only numbers inside cards are allowed to centre.
Item {
    id: root

    property string kicker: ""
    property string title: ""
    property bool showBack: false
    property string trailingGlyph: ""
    signal back()
    signal trailing()

    // Without a width the trailing button anchors to parent.right at x=0 and
    // lands on top of the back button, which is how one page shipped a broken
    // header. Default to the parent so the failure cannot repeat silently; a
    // page that needs another width just assigns it.
    width: parent ? parent.width : 0
    height: Math.max(col.height, units.gu(5))

    Item {
        id: backBtn
        visible: root.showBack
        width: visible ? units.gu(4) : 0
        height: units.gu(4)
        anchors.left: parent.left
        anchors.top: parent.top

        Glyph {
            anchors.centerIn: parent
            name: "back"
            size: units.gu(2.75)
            color: Pulse.text
        }
        MouseArea {
            anchors.fill: parent
            anchors.margins: -Pulse.s
            onClicked: root.back()
        }
    }

    Column {
        id: col
        anchors.left: backBtn.visible ? backBtn.right : parent.left
        anchors.leftMargin: backBtn.visible ? Pulse.s : 0
        anchors.right: trailingBtn.visible ? trailingBtn.left : parent.right
        anchors.rightMargin: Pulse.m
        anchors.top: parent.top
        spacing: units.dp(2)

        Label {
            visible: root.kicker.length > 0
            text: root.kicker.toUpperCase()
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.micro
            font.weight: Font.DemiBold
            font.letterSpacing: units.dp(1.4)
        }

        Label {
            width: parent.width
            text: root.title
            color: Pulse.text
            font.family: Pulse.face
            font.pixelSize: Pulse.headline
            font.weight: Font.Light
            elide: Text.ElideRight
        }
    }

    Item {
        id: trailingBtn
        visible: root.trailingGlyph.length > 0
        width: visible ? units.gu(4.5) : 0
        height: units.gu(4.5)
        anchors.right: parent.right
        anchors.top: parent.top

        Rectangle {
            anchors.fill: parent
            radius: width / 2
            color: Pulse.card
        }
        Glyph {
            anchors.centerIn: parent
            name: root.trailingGlyph
            size: units.gu(2.5)
            color: Pulse.text
        }
        MouseArea {
            anchors.fill: parent
            onClicked: root.trailing()
        }
    }
}
