import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/Fmt.js" as Fmt
import "../store"

// Day picker shared by Today and Sleep. Forward is disabled on today; the
// daemon has no future data and a dead end reads worse than a dim arrow.
Item {
    id: root

    height: units.gu(4)

    Rectangle {
        anchors.fill: parent
        radius: Pulse.radiusPill
        color: Pulse.card
    }

    Item {
        id: prev
        width: units.gu(4)
        height: parent.height
        anchors.left: parent.left
        Glyph {
            anchors.centerIn: parent
            name: "back"
            size: units.gu(2)
            color: Pulse.text
        }
        MouseArea {
            anchors.fill: parent
            onClicked: Store.setDate(Fmt.shiftIso(Store.date, -1))
        }
    }

    Label {
        anchors.centerIn: parent
        text: Fmt.prettyDate(Store.date)
        color: Pulse.text
        font.family: Pulse.face
        font.pixelSize: Pulse.caption
        font.weight: Font.DemiBold
    }

    Item {
        id: next
        width: units.gu(4)
        height: parent.height
        anchors.right: parent.right
        opacity: Store.isToday ? 0.25 : 1
        Glyph {
            anchors.centerIn: parent
            name: "chevron"
            size: units.gu(2)
            color: Pulse.text
        }
        MouseArea {
            anchors.fill: parent
            enabled: !Store.isToday
            onClicked: Store.setDate(Fmt.shiftIso(Store.date, 1))
        }
    }
}
