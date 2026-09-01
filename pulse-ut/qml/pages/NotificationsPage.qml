import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt

// Mirror of what pulsed forwarded to the watch. Read-only on purpose: this is
// a diagnostic surface, the phone already owns the real notification centre.
Item {
    id: page

    signal back()

    readonly property var list: Store.notifications || []
    readonly property bool waiting: Store.notifications === null &&
                                    (Store.notificationsLoading || !Store.everAnswered)

    Component.onCompleted: Store.loadNotifications()

    function sourceGlyph(src) {
        if (src === "waydroid") return "android";
        if (src === "call") return "phone";
        return "desktop";
    }
    function sourceColor(src) {
        if (src === "waydroid") return Pulse.mint;
        if (src === "call") return Pulse.ringHr;
        return Pulse.accent;
    }

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            showBack: true
            kicker: "Sent to the watch"
            title: "Notifications"
            trailingGlyph: "sync"
            onTrailing: Store.loadNotifications()
            onBack: page.back()
        }

        Row {
            width: parent.width
            spacing: Pulse.s
            visible: !page.waiting && page.list.length > 0

            Repeater {
                model: [
                    { key: "freedesktop", label: "Desktop" },
                    { key: "waydroid", label: "Waydroid" },
                    { key: "call", label: "Calls" }
                ]

                delegate: Rectangle {
                    id: tile
                    readonly property int count: {
                        var n = 0;
                        for (var i = 0; i < page.list.length; i++)
                            if (page.list[i].source === modelData.key) n++;
                        return n;
                    }
                    width: (parent.width - 2 * Pulse.s) / 3
                    height: units.gu(7)
                    radius: Pulse.radiusTile
                    color: Pulse.card

                    Column {
                        anchors.centerIn: parent
                        spacing: units.dp(2)
                        Label {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: "" + tile.count
                            color: page.sourceColor(modelData.key)
                            font.family: Pulse.face
                            font.pixelSize: Pulse.title
                            font.weight: Font.DemiBold
                        }
                        Label {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: modelData.label
                            color: Pulse.textDim
                            font.family: Pulse.face
                            font.pixelSize: Pulse.micro
                        }
                    }
                }
            }
        }

        Column {
            width: parent.width
            spacing: Pulse.m
            visible: page.waiting

            Repeater {
                model: 5
                delegate: Skeleton {
                    width: parent.width
                    height: units.gu(8)
                    radius: Pulse.radiusCard
                    delay: index * 110
                }
            }
        }

        Card {
            width: parent.width
            padding: page.list.length > 0 ? Pulse.s : Pulse.l
            spacing: 0
            visible: !page.waiting

            EmptyState {
                width: parent.width
                visible: page.list.length === 0
                glyph: "bell"
                title: Store.online ? "Nothing forwarded yet" : "Pulse daemon offline"
                hint: Store.online
                      ? "Notifications appear here the moment pulsed relays one to the watch. Check that forwarding is enabled on the Device tab."
                      : "Pulse cannot reach pulsed on 127.0.0.1:21830."
            }

            Repeater {
                model: page.list

                delegate: Item {
                    width: parent.width
                    height: units.gu(9)
                    opacity: modelData.removed ? 0.45 : 1

                    Rectangle {
                        id: chip
                        anchors.left: parent.left
                        anchors.leftMargin: Pulse.m
                        anchors.top: parent.top
                        anchors.topMargin: Pulse.m
                        width: units.gu(4)
                        height: units.gu(4)
                        radius: Pulse.radiusPill
                        color: Pulse.alpha(page.sourceColor(modelData.source), 0.16)

                        Glyph {
                            anchors.centerIn: parent
                            name: page.sourceGlyph(modelData.source)
                            size: units.gu(2.25)
                            color: page.sourceColor(modelData.source)
                        }
                    }

                    Column {
                        anchors.left: chip.right
                        anchors.leftMargin: Pulse.m
                        anchors.right: parent.right
                        anchors.rightMargin: Pulse.m
                        anchors.top: parent.top
                        anchors.topMargin: Pulse.m
                        spacing: units.dp(2)

                        Row {
                            width: parent.width
                            spacing: Pulse.xs

                            Label {
                                text: modelData.appName && modelData.appName.length
                                      ? modelData.appName
                                      : (modelData.appId || modelData.source)
                                color: Pulse.textDim
                                font.family: Pulse.face
                                font.pixelSize: Pulse.micro
                                font.weight: Font.DemiBold
                                font.letterSpacing: units.dp(0.8)
                            }
                            Label {
                                text: "\u00b7 " + Fmt.relative(modelData.tsMs)
                                color: Pulse.textDim
                                font.family: Pulse.face
                                font.pixelSize: Pulse.micro
                            }
                            Label {
                                visible: modelData.removed
                                text: "\u00b7 dismissed"
                                color: Pulse.ringCal
                                font.family: Pulse.face
                                font.pixelSize: Pulse.micro
                            }
                        }

                        Label {
                            width: parent.width
                            elide: Text.ElideRight
                            text: modelData.title && modelData.title.length ? modelData.title : "(no title)"
                            color: Pulse.text
                            font.family: Pulse.face
                            font.pixelSize: Pulse.body
                            font.weight: Font.DemiBold
                        }

                        Label {
                            width: parent.width
                            elide: Text.ElideRight
                            maximumLineCount: 2
                            wrapMode: Text.WordWrap
                            text: modelData.body || ""
                            color: Pulse.textDim
                            font.family: Pulse.face
                            font.pixelSize: Pulse.caption
                        }
                    }

                    Rectangle {
                        anchors.left: chip.left
                        anchors.right: parent.right
                        anchors.rightMargin: Pulse.m
                        anchors.bottom: parent.bottom
                        height: units.dp(1)
                        color: Pulse.hairline
                        visible: index < page.list.length - 1
                    }
                }
            }
        }
    }
}
