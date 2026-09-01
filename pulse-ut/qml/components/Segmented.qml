import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Compact choice control: [{key, label}, ...]
Rectangle {
    id: root

    property var options: []
    property string current: ""
    signal picked(string key)

    readonly property int index: {
        for (var i = 0; i < (options || []).length; i++)
            if (options[i].key === current) return i;
        return -1;
    }
    readonly property real cell: options && options.length > 0 ? width / options.length : 0

    height: units.gu(4.25)
    radius: Pulse.radiusPill
    color: Pulse.cardAlt

    Rectangle {
        visible: root.index >= 0
        width: root.cell - units.dp(4)
        height: parent.height - units.dp(4)
        y: units.dp(2)
        x: units.dp(2) + root.index * root.cell
        radius: Pulse.radiusPill - units.dp(1)
        color: Pulse.accent
        Behavior on x { NumberAnimation { duration: Pulse.med; easing.type: Easing.OutQuart } }
    }

    Row {
        anchors.fill: parent

        Repeater {
            model: root.options || []

            delegate: Item {
                width: root.cell
                height: root.height

                Label {
                    anchors.centerIn: parent
                    text: modelData.label
                    color: index === root.index ? Pulse.onAccent : Pulse.textDim
                    font.family: Pulse.face
                    font.pixelSize: Pulse.caption
                    font.weight: index === root.index ? Font.DemiBold : Font.Normal
                    Behavior on color { ColorAnimation { duration: Pulse.fast } }
                }

                MouseArea {
                    anchors.fill: parent
                    onClicked: root.picked(modelData.key)
                }
            }
        }
    }
}
