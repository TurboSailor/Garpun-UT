import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/Fmt.js" as Fmt
import "../js/I18n.js" as I18n

Item {
    id: root

    property var device: null
    property bool divider: false
    property bool active: false
    signal connectRequested()
    signal forgetRequested()

    height: units.gu(8)

    Rectangle {
        id: dot
        anchors.left: parent.left
        anchors.leftMargin: Pulse.m
        anchors.verticalCenter: parent.verticalCenter
        width: units.gu(1)
        height: width
        radius: width / 2
        color: root.device && root.device.connected ? Pulse.mint : Pulse.textDim
    }

    Column {
        anchors.left: dot.right
        anchors.leftMargin: Pulse.m
        anchors.right: actions.left
        anchors.rightMargin: Pulse.s
        anchors.verticalCenter: parent.verticalCenter
        spacing: units.dp(2)

        Label {
            width: parent.width
            elide: Text.ElideRight
            text: root.device
                  ? (root.device.name && root.device.name.length ? root.device.name : root.device.address)
                  : ""
            color: Pulse.text
            font.family: Pulse.face
            font.pixelSize: Pulse.body
            font.weight: Font.DemiBold
        }

        Label {
            width: parent.width
            elide: Text.ElideRight
            text: {
                if (!root.device) return "";
                var bits = [];
                if (root.device.model && root.device.model.length) bits.push(root.device.model);
                bits.push(root.device.connected ? I18n.t("device.row_connected") : I18n.t("device.row_offline"));
                if (root.device.lastSyncMs > 0)
                    bits.push(I18n.t("status.synced_relative", [Fmt.relative(root.device.lastSyncMs)]));
                return bits.join(" \u00b7 ");
            }
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.caption
        }
    }

    Row {
        id: actions
        anchors.right: parent.right
        anchors.rightMargin: Pulse.s
        anchors.verticalCenter: parent.verticalCenter
        spacing: Pulse.xs

        Rectangle {
            visible: !(root.device && root.device.connected)
            width: units.gu(4.5)
            height: units.gu(4.5)
            radius: width / 2
            color: connectTap.pressed ? Pulse.alpha(Pulse.accent, 0.22) : Pulse.cardAlt
            Glyph {
                anchors.centerIn: parent
                name: "bluetooth"
                size: units.gu(2.25)
                color: Pulse.accent
            }
            MouseArea {
                id: connectTap
                anchors.fill: parent
                onClicked: root.connectRequested()
            }
        }

        Rectangle {
            width: units.gu(4.5)
            height: units.gu(4.5)
            radius: width / 2
            color: forgetTap.pressed ? Pulse.alpha(Pulse.ringHr, 0.22) : Pulse.cardAlt
            Glyph {
                anchors.centerIn: parent
                name: "trash"
                size: units.gu(2.25)
                color: Pulse.ringHr
            }
            MouseArea {
                id: forgetTap
                anchors.fill: parent
                onClicked: root.forgetRequested()
            }
        }
    }

    Rectangle {
        visible: root.divider
        anchors.left: dot.left
        anchors.right: actions.right
        anchors.bottom: parent.bottom
        height: units.dp(1)
        color: Pulse.hairline
    }
}
