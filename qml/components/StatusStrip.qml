import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/Fmt.js" as Fmt
import "../store"

// Always-visible truth about the link: daemon, watch, battery, last sync.
// This is what makes an empty dashboard understandable instead of broken.
Rectangle {
    id: root

    property bool showSync: true

    readonly property bool daemonDown: !Store.online
    readonly property color dotColor: daemonDown ? Pulse.ringHr
                                    : Store.connected ? Pulse.mint
                                    : Pulse.ringCal

    height: units.gu(6)
    radius: Pulse.radiusTile
    color: Pulse.card

    Rectangle {
        id: dot
        anchors.left: parent.left
        anchors.leftMargin: Pulse.m
        anchors.verticalCenter: parent.verticalCenter
        width: units.gu(1)
        height: width
        radius: width / 2
        color: root.dotColor

        SequentialAnimation on opacity {
            running: Store.syncing
            loops: Animation.Infinite
            NumberAnimation { to: 0.25; duration: 480 }
            NumberAnimation { to: 1.0; duration: 480 }
        }
    }

    Column {
        anchors.left: dot.right
        anchors.leftMargin: Pulse.m
        anchors.right: syncBtn.visible ? syncBtn.left : parent.right
        anchors.rightMargin: Pulse.m
        anchors.verticalCenter: parent.verticalCenter
        spacing: units.dp(1)

        Label {
            width: parent.width
            elide: Text.ElideRight
            font.family: Pulse.face
            font.pixelSize: Pulse.body
            font.weight: Font.DemiBold
            color: Pulse.text
            text: root.daemonDown ? "Pulse daemon offline"
                : !Store.device ? "No watch paired"
                : (Store.device.name && Store.device.name.length ? Store.device.name : Store.device.address)
        }

        Label {
            width: parent.width
            elide: Text.ElideRight
            font.family: Pulse.face
            font.pixelSize: Pulse.caption
            color: Pulse.textDim
            text: {
                if (root.daemonDown) return "Start pulsed to see your data";
                if (!Store.device) return "Open Device to scan and pair";
                if (Store.syncing) {
                    var p = Store.progress;
                    if (p && p.total > 0)
                        return "Syncing file " + (p.fileIndex + 1) + " \u00b7 " +
                               Math.round(100 * p.received / p.total) + "%";
                    return "Syncing\u2026";
                }
                var bits = [];
                bits.push(Store.connected ? (Store.initialized ? "Connected" : "Connecting\u2026") : "Disconnected");
                if (Store.battery >= 0) bits.push(Store.battery + "%");
                if (Store.device.lastSyncMs > 0) bits.push("synced " + Fmt.relative(Store.device.lastSyncMs));
                return bits.join(" \u00b7 ");
            }
        }
    }

    Item {
        id: syncBtn
        visible: root.showSync && Store.online && !!Store.device
        width: visible ? units.gu(4.5) : 0
        height: units.gu(4.5)
        anchors.right: parent.right
        anchors.rightMargin: Pulse.s
        anchors.verticalCenter: parent.verticalCenter

        Rectangle {
            anchors.fill: parent
            radius: width / 2
            color: tap.pressed ? Pulse.alpha(Pulse.accent, 0.22) : Pulse.cardAlt
        }

        Glyph {
            id: syncIcon
            anchors.centerIn: parent
            name: "sync"
            size: units.gu(2.25)
            color: Store.connected ? Pulse.accent : Pulse.textDim

            RotationAnimator on rotation {
                running: Store.syncing
                loops: Animation.Infinite
                from: 0; to: 360; duration: 1100
            }
        }

        MouseArea {
            id: tap
            anchors.fill: parent
            enabled: Store.connected && !Store.syncing
            onClicked: Store.startSync()
        }
    }

    // sync progress hairline pinned to the bottom edge
    Rectangle {
        visible: Store.syncing
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: Pulse.m
        height: units.dp(3)
        radius: height / 2
        color: Pulse.cardAlt

        Rectangle {
            height: parent.height
            radius: parent.radius
            color: Pulse.accent
            width: {
                var p = Store.progress;
                if (!p || !p.total) return 0;
                return Math.max(parent.height, parent.width * Math.min(1, p.received / p.total));
            }
            Behavior on width { NumberAnimation { duration: Pulse.med } }
        }
    }
}
