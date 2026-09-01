import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/Fmt.js" as Fmt

Item {
    id: root

    property var workout: null
    property bool divider: false
    signal clicked()

    readonly property real seconds: workout && workout.endMs > workout.startMs
                                    ? (workout.endMs - workout.startMs) / 1000 : 0

    height: units.gu(7)

    Rectangle {
        anchors.fill: parent
        anchors.margins: units.dp(1)
        radius: Pulse.radiusTile
        color: Pulse.text
        opacity: tap.pressed ? 0.05 : 0
        Behavior on opacity { NumberAnimation { duration: Pulse.fast } }
    }

    Glyph {
        id: icon
        anchors.left: parent.left
        anchors.leftMargin: Pulse.m
        anchors.verticalCenter: parent.verticalCenter
        name: {
            if (!root.workout) return "timer";
            if (root.workout.sport !== undefined && root.workout.sport > 0)
                return Pulse.sportGlyph(root.workout.sport);
            return Pulse.activityGlyph(root.workout.kind);
        }
        size: units.gu(2.5)
        color: Pulse.accent
    }

    Column {
        anchors.left: icon.right
        anchors.leftMargin: Pulse.m
        anchors.right: trailing.left
        anchors.rightMargin: Pulse.s
        anchors.verticalCenter: parent.verticalCenter
        spacing: units.dp(2)

        Label {
            width: parent.width
            elide: Text.ElideRight
            text: Fmt.workoutTitle(root.workout)
            color: Pulse.text
            font.family: Pulse.face
            font.pixelSize: Pulse.body
            font.weight: Font.DemiBold
        }
        Label {
            width: parent.width
            elide: Text.ElideRight
            text: root.workout
                  ? Fmt.dateShort(root.workout.startMs) + " \u00b7 " + Fmt.timeOfDay(root.workout.startMs) +
                    " \u00b7 " + Fmt.clock(root.seconds)
                  : ""
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.caption
        }
    }

    Glyph {
        id: trailing
        anchors.right: parent.right
        anchors.rightMargin: Pulse.m
        anchors.verticalCenter: parent.verticalCenter
        name: "chevron"
        size: units.gu(2)
        color: Pulse.textDim
    }

    Rectangle {
        visible: root.divider
        anchors.left: icon.left
        anchors.right: trailing.right
        anchors.bottom: parent.bottom
        height: units.dp(1)
        color: Pulse.hairline
    }

    MouseArea {
        id: tap
        anchors.fill: parent
        onClicked: root.clicked()
    }
}
