import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../js/I18n.js" as I18n

// Modal only because BlueZ genuinely blocks on the answer. Handles both
// pairing kinds the daemon can report.
Item {
    id: root

    readonly property var req: Store.pairing
    readonly property bool active: !!(req && req.pending)
    readonly property bool needsEntry: req && req.kind === "passkey"

    property string entry: ""

    visible: opacity > 0
    opacity: active ? 1 : 0
    Behavior on opacity { NumberAnimation { duration: Pulse.med } }

    onActiveChanged: if (active) entry = ""

    Rectangle {
        anchors.fill: parent
        color: Qt.rgba(0, 0, 0, 0.72)
        MouseArea { anchors.fill: parent }
    }

    Rectangle {
        id: sheet
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.verticalCenter: parent.verticalCenter
        width: Math.min(parent.width - 2 * Pulse.l, units.gu(44))
        height: col.height + 2 * Pulse.xl
        radius: Pulse.radiusCard
        color: Pulse.card
        anchors.verticalCenterOffset: root.active ? 0 : units.gu(4)
        Behavior on anchors.verticalCenterOffset { NumberAnimation { duration: Pulse.med; easing.type: Easing.OutQuart } }

        Column {
            id: col
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: Pulse.xl
            spacing: Pulse.l

            Label {
                text: root.needsEntry ? I18n.t("pairing.enter_code") : I18n.t("pairing.confirm")
                color: Pulse.text
                font.family: Pulse.face
                font.pixelSize: Pulse.subtitle
                font.weight: Font.DemiBold
                width: parent.width
                wrapMode: Text.WordWrap
            }

            Label {
                text: root.req && root.req.address ? root.req.address : ""
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }

            // confirm: the daemon already knows the number
            Label {
                visible: !root.needsEntry
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: root.req && root.req.passkey !== undefined ? ("" + root.req.passkey) : ""
                color: Pulse.accent
                font.family: Pulse.face
                font.pixelSize: Pulse.display
                font.weight: Font.Light
                font.letterSpacing: units.dp(6)
            }

            // passkey: six slots the keypad fills
            Row {
                visible: root.needsEntry
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Pulse.s

                Repeater {
                    model: 6
                    delegate: Rectangle {
                        width: units.gu(4.5)
                        height: units.gu(6)
                        radius: Pulse.radiusPill
                        color: Pulse.cardAlt
                        border.width: index === root.entry.length ? units.dp(2) : 0
                        border.color: Pulse.accent

                        Label {
                            anchors.centerIn: parent
                            text: index < root.entry.length ? root.entry.charAt(index) : ""
                            color: Pulse.text
                            font.family: Pulse.face
                            font.pixelSize: Pulse.title
                            font.weight: Font.DemiBold
                        }
                    }
                }
            }

            Grid {
                id: pad
                visible: root.needsEntry
                anchors.horizontalCenter: parent.horizontalCenter
                columns: 3
                spacing: Pulse.s

                readonly property real cell: (col.width - 2 * spacing) / 3

                Repeater {
                    model: ["1", "2", "3", "4", "5", "6", "7", "8", "9", "", "0", "\u232b"]

                    delegate: Rectangle {
                        width: pad.cell
                        height: units.gu(5.5)
                        radius: Pulse.radiusPill
                        color: modelData.length === 0 ? "transparent"
                             : keyTap.pressed ? Pulse.alpha(Pulse.accent, 0.22) : Pulse.cardAlt

                        Label {
                            anchors.centerIn: parent
                            text: modelData
                            color: Pulse.text
                            font.family: Pulse.face
                            font.pixelSize: Pulse.subtitle
                            font.weight: Font.DemiBold
                        }

                        MouseArea {
                            id: keyTap
                            anchors.fill: parent
                            enabled: modelData.length > 0
                            onClicked: {
                                if (modelData === "\u232b")
                                    root.entry = root.entry.slice(0, -1);
                                else if (root.entry.length < 6)
                                    root.entry += modelData;
                            }
                        }
                    }
                }
            }

            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Pulse.m

                PillButton {
                    text: I18n.t("action.cancel")
                    kind: "ghost"
                    onClicked: Store.replyPairing({ cancel: true })
                }

                PillButton {
                    text: root.needsEntry ? I18n.t("action.pair") : I18n.t("action.confirm")
                    kind: "primary"
                    enabledLook: !root.needsEntry || root.entry.length === 6
                    onClicked: {
                        if (root.needsEntry)
                            Store.replyPairing({ passkey: parseInt(root.entry, 10) });
                        else
                            Store.replyPairing({ confirm: true });
                    }
                }
            }
        }
    }
}
