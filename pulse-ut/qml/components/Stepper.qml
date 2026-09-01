import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Minus / value / plus. Press-and-hold accelerates, otherwise reaching 12 000
// steps from 8 000 would be a chore.
Row {
    id: root

    property real value: 0
    property real step: 1
    property real minimum: 0
    property real maximum: 1000000
    property string display: "" + value
    signal changed(real value)

    spacing: Pulse.s

    function bump(dir) {
        var next = Math.round((value + dir * step) / step) * step;
        if (next < minimum) next = minimum;
        if (next > maximum) next = maximum;
        if (next !== value) root.changed(next);
    }

    Component {
        id: knobComponent
        Rectangle {
            property string glyph: "plus"
            property int dir: 1
            width: units.gu(4)
            height: units.gu(4)
            radius: Pulse.radiusPill
            color: hold.pressed ? Pulse.alpha(Pulse.accent, 0.22) : Pulse.cardAlt

            Glyph {
                anchors.centerIn: parent
                name: parent.glyph
                size: units.gu(2)
                color: Pulse.text
            }

            MouseArea {
                id: hold
                anchors.fill: parent
                onClicked: root.bump(parent.dir)
                onPressAndHold: repeatTimer.start()
                onReleased: repeatTimer.stop()
                onCanceled: repeatTimer.stop()
                Timer {
                    id: repeatTimer
                    interval: 90
                    repeat: true
                    onTriggered: root.bump(hold.parent.dir * 5)
                }
            }
        }
    }

    Loader {
        sourceComponent: knobComponent
        onLoaded: { item.glyph = "minus"; item.dir = -1; }
        anchors.verticalCenter: parent.verticalCenter
    }

    Label {
        width: units.gu(9)
        horizontalAlignment: Text.AlignHCenter
        anchors.verticalCenter: parent.verticalCenter
        text: root.display
        color: Pulse.text
        font.family: Pulse.face
        font.pixelSize: Pulse.subtitle
        font.weight: Font.DemiBold
    }

    Loader {
        sourceComponent: knobComponent
        onLoaded: { item.glyph = "plus"; item.dir = 1; }
        anchors.verticalCenter: parent.verticalCenter
    }
}
