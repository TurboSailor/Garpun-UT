import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/I18n.js" as I18n

Item {
    id: root

    property var entry: null
    property bool divider: false
    signal pairRequested()

    // RSSI is roughly -30 (touching) to -100 (far); four bars is enough.
    readonly property int bars: {
        if (!entry || entry.rssi === undefined) return 0;
        var r = entry.rssi;
        if (r >= -55) return 4;
        if (r >= -70) return 3;
        if (r >= -85) return 2;
        return 1;
    }

    height: units.gu(7.5)

    Row {
        id: sig
        anchors.left: parent.left
        anchors.leftMargin: Pulse.m
        anchors.verticalCenter: parent.verticalCenter
        height: units.dp(14)
        spacing: units.dp(2)

        Repeater {
            model: 4
            delegate: Item {
                width: units.dp(3)
                height: units.dp(14)
                Rectangle {
                    anchors.bottom: parent.bottom
                    width: parent.width
                    height: units.dp(5) + index * units.dp(3)
                    radius: width / 2
                    color: index < root.bars ? Pulse.accent : Pulse.cardAlt
                }
            }
        }
    }

    Column {
        anchors.left: sig.right
        anchors.leftMargin: Pulse.m
        anchors.right: pairBtn.left
        anchors.rightMargin: Pulse.s
        anchors.verticalCenter: parent.verticalCenter
        spacing: units.dp(2)

        Row {
            spacing: Pulse.xs
            Label {
                text: root.entry
                      ? (root.entry.name && root.entry.name.length ? root.entry.name : I18n.t("device.nothing_found"))
                      : ""
                color: Pulse.text
                font.family: Pulse.face
                font.pixelSize: Pulse.body
                font.weight: Font.DemiBold
                anchors.verticalCenter: parent.verticalCenter
            }
            Rectangle {
                visible: !!(root.entry && root.entry.garmin)
                anchors.verticalCenter: parent.verticalCenter
                width: tag.width + Pulse.s
                height: tag.height + units.dp(4)
                radius: height / 2
                color: Pulse.alpha(Pulse.accent, 0.18)
                Label {
                    id: tag
                    anchors.centerIn: parent
                    text: "GARMIN"
                    color: Pulse.accent
                    font.family: Pulse.face
                    font.pixelSize: Pulse.micro
                    font.weight: Font.DemiBold
                    font.letterSpacing: units.dp(0.8)
                }
            }
        }

        Label {
            width: parent.width
            elide: Text.ElideRight
            text: root.entry
                  ? root.entry.address + (root.entry.rssi !== undefined ? "  \u00b7  " + root.entry.rssi + " dBm" : "")
                  : ""
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.caption
        }
    }

    PillButton {
        id: pairBtn
        anchors.right: parent.right
        anchors.rightMargin: Pulse.s
        anchors.verticalCenter: parent.verticalCenter
        text: root.entry && root.entry.paired ? I18n.t("action.paired") : I18n.t("action.pair")
        kind: root.entry && root.entry.paired ? "ghost" : "primary"
        enabledLook: !(root.entry && root.entry.paired)
        implicitHeight: units.gu(4.25)
        onClicked: root.pairRequested()
    }

    Rectangle {
        visible: root.divider
        anchors.left: sig.left
        anchors.right: pairBtn.right
        anchors.bottom: parent.bottom
        height: units.dp(1)
        color: Pulse.hairline
    }
}
