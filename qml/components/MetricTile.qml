import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Dashboard tile. The card body carries a bottom-up wash whose height tracks
// the goal factor, and the hairline bar underneath states it precisely.
Rectangle {
    id: root

    property string label: ""
    property string value: "\u2013"
    property string unit: ""
    property string caption: ""
    property string glyph: ""
    property real factor: 0
    property color hue: Pulse.accent
    property bool loading: false
    signal clicked()

    readonly property real clamped: Pulse.clamp01(factor)

    radius: Pulse.radiusTile
    color: Pulse.card
    // Floored so a row of tiles stays level whether or not the caption has
    // something to say.
    implicitHeight: Math.max(units.gu(13), body.height + 2 * Pulse.m + bar.height + Pulse.s)

    // progress wash, rotated so the gradient grows from the bottom edge
    Rectangle {
        anchors.fill: parent
        radius: parent.radius
        rotation: 180
        visible: root.clamped > 0 && !root.loading
        gradient: Gradient {
            GradientStop { position: 0.0; color: Pulse.alpha(root.hue, 0.20) }
            GradientStop { position: Math.max(0.04, root.clamped); color: Pulse.alpha(root.hue, 0.0) }
            GradientStop { position: 1.0; color: Pulse.alpha(root.hue, 0.0) }
        }
    }

    Column {
        id: body
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: Pulse.m
        spacing: Pulse.xs

        Row {
            spacing: Pulse.xs
            Glyph {
                name: root.glyph
                size: units.gu(2)
                color: root.hue
                anchors.verticalCenter: parent.verticalCenter
            }
            Label {
                text: root.label
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
                anchors.verticalCenter: parent.verticalCenter
            }
        }

        Item { width: 1; height: Pulse.xs }

        Skeleton {
            visible: root.loading
            width: units.gu(8)
            height: Pulse.title
        }

        Row {
            visible: !root.loading
            spacing: units.dp(3)
            Label {
                text: root.value
                color: Pulse.text
                font.family: Pulse.face
                font.pixelSize: Pulse.title
                font.weight: Font.DemiBold
                anchors.baseline: unitLabel.baseline
            }
            Label {
                id: unitLabel
                text: root.unit
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }
        }

        Label {
            visible: !root.loading
            text: root.caption
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.micro
            elide: Text.ElideRight
            width: parent.width
        }
    }

    ProgressLine {
        id: bar
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: Pulse.m
        anchors.bottomMargin: Pulse.m
        factor: root.loading ? 0 : root.factor
        hue: root.hue
    }

    Rectangle {
        anchors.fill: parent
        radius: parent.radius
        color: Pulse.text
        opacity: tap.pressed ? 0.06 : 0
        Behavior on opacity { NumberAnimation { duration: Pulse.fast } }
    }

    MouseArea {
        id: tap
        anchors.fill: parent
        onClicked: root.clicked()
    }
}
