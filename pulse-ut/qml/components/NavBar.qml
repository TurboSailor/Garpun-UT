import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

Item {
    id: root

    property var tabs: []
    property int current: 0
    signal picked(int index)

    height: units.gu(7.5)

    Rectangle {
        anchors.fill: parent
        color: Pulse.bg
    }

    Rectangle {
        anchors.top: parent.top
        anchors.left: parent.left
        anchors.right: parent.right
        height: units.dp(1)
        color: Pulse.hairline
    }

    Row {
        anchors.fill: parent

        Repeater {
            model: root.tabs

            delegate: Item {
                id: tab
                width: root.width / Math.max(1, root.tabs.length)
                height: root.height

                readonly property bool active: index === root.current

                Rectangle {
                    anchors.horizontalCenter: parent.horizontalCenter
                    anchors.top: parent.top
                    width: units.gu(3)
                    height: units.dp(3)
                    radius: height / 2
                    color: Pulse.accent
                    opacity: tab.active ? 1 : 0
                    Behavior on opacity { NumberAnimation { duration: Pulse.med } }
                }

                Column {
                    anchors.centerIn: parent
                    spacing: units.dp(3)

                    Glyph {
                        anchors.horizontalCenter: parent.horizontalCenter
                        name: modelData.glyph
                        size: units.gu(2.75)
                        color: tab.active ? Pulse.accent : Pulse.textDim
                        weight: tab.active ? 2.2 : 1.8
                    }

                    Label {
                        anchors.horizontalCenter: parent.horizontalCenter
                        text: modelData.label
                        color: tab.active ? Pulse.text : Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.micro
                        font.weight: tab.active ? Font.DemiBold : Font.Normal
                    }
                }

                MouseArea {
                    anchors.fill: parent
                    onClicked: root.picked(index)
                }
            }
        }
    }
}
