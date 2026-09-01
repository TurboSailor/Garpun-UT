import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt

Item {
    id: page

    signal openNotifications()

    readonly property var st: Store.settings
    readonly property var paired: Store.devices || []
    // A device already paired should not be offered again in the scan list.
    readonly property var found: {
        var out = [];
        var list = Store.scan || [];
        for (var i = 0; i < list.length; i++) {
            var seen = false;
            for (var j = 0; j < paired.length; j++)
                if (paired[j].address === list[i].address) seen = true;
            if (!seen) out.push(list[i]);
        }
        return out;
    }

    function set(key, value) { Store.setSetting(key, value); }

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            kicker: Store.daemonVersion.length > 0 ? "pulsed " + Store.daemonVersion : "pulsed"
            title: "Device"
            trailingGlyph: "bell"
            trailingBadge: Store.notifications ? Store.notifications.length : 0
            onTrailing: page.openNotifications()
        }

        StatusStrip { width: parent.width; showSync: false }

        // ---- actions ---------------------------------------------------
        Flow {
            width: parent.width
            spacing: Pulse.s

            PillButton {
                text: Store.connected ? "Disconnect" : "Connect"
                glyph: "bluetooth"
                kind: Store.connected ? "ghost" : "primary"
                enabledLook: Store.online && !!Store.device
                onClicked: {
                    if (Store.connected) Store.disconnectDevice();
                    else if (Store.device) Store.connectDevice(Store.device.address);
                }
            }

            PillButton {
                text: Store.syncing ? "Syncing" : "Sync"
                glyph: "sync"
                busy: Store.syncing
                enabledLook: Store.connected && !Store.syncing
                onClicked: Store.startSync()
            }

            PillButton {
                text: Store.ringing ? "Stop ringing" : "Find my watch"
                glyph: Store.ringing ? "close" : "search"
                kind: Store.ringing ? "danger" : "ghost"
                enabledLook: Store.connected
                onClicked: {
                    if (Store.ringing) Store.cancelFindWatch();
                    else Store.findWatch(30);
                }
            }
        }

        // ---- sync progress -----------------------------------------------
        Card {
            width: parent.width
            visible: Store.syncing

            Label {
                text: "Transferring files"
                color: Pulse.text
                font.family: Pulse.face
                font.pixelSize: Pulse.body
                font.weight: Font.DemiBold
            }

            ProgressLine {
                width: parent.width
                height: units.dp(6)
                hue: Pulse.accent
                factor: {
                    var p = Store.progress;
                    return p && p.total > 0 ? p.received / p.total : 0;
                }
            }

            Label {
                text: {
                    var p = Store.progress;
                    if (!p) return "";
                    return "File " + (p.fileIndex + 1) + " \u00b7 " + Fmt.thousands(p.received) +
                           " of " + Fmt.thousands(p.total) + " bytes \u00b7 " +
                           Fmt.thousands(p.remaining) + " files left";
                }
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }
        }

        // ---- paired ----------------------------------------------------
        SectionTitle { width: parent.width; text: "Paired" }

        Card {
            width: parent.width
            padding: page.paired.length > 0 ? Pulse.s : Pulse.l
            spacing: 0

            EmptyState {
                width: parent.width
                visible: page.paired.length === 0
                glyph: "watch"
                title: Store.online ? "No watch paired yet" : "Pulse daemon offline"
                hint: Store.online
                      ? "Put the Garmin in pairing mode, then scan below."
                      : "Start pulsed \u2014 Bluetooth is handled by the daemon, not this app."
            }

            Repeater {
                model: page.paired
                delegate: DeviceRow {
                    width: parent.width
                    device: modelData
                    divider: index < page.paired.length - 1
                    active: Store.device && Store.device.address === modelData.address
                    onConnectRequested: Store.connectDevice(modelData.address)
                    onForgetRequested: Store.forgetDevice(modelData.address)
                }
            }
        }

        // ---- scan ------------------------------------------------------
        SectionTitle {
            width: parent.width
            text: "Nearby"
            action: Store.scanning ? "Stop" : "Scan"
            onActionTriggered: {
                if (Store.scanning) Store.stopScan();
                else Store.startScan();
            }
        }

        Card {
            width: parent.width
            padding: page.found.length > 0 ? Pulse.s : Pulse.l
            spacing: 0

            EmptyState {
                width: parent.width
                visible: page.found.length === 0
                glyph: "search"
                title: Store.scanning ? "Scanning\u2026" : "Nothing found yet"
                hint: Store.scanning
                      ? "Keep the watch awake and close to the phone."
                      : "Tap Scan and hold the watch nearby. Garmin devices are flagged automatically."
                action: Store.scanning || !Store.online ? "" : "Scan"
                onActionTriggered: Store.startScan()
            }

            Repeater {
                model: page.found
                delegate: ScanRow {
                    width: parent.width
                    entry: modelData
                    divider: index < page.found.length - 1
                    onPairRequested: Store.pairDevice(modelData.address)
                }
            }
        }

        // ---- appearance -------------------------------------------------
        SectionTitle { width: parent.width; text: "Appearance" }

        Card {
            width: parent.width

            Label {
                text: "Theme"
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }

            Segmented {
                width: parent.width
                options: [
                    { key: "system", label: "System" },
                    { key: "light", label: "Light" },
                    { key: "dark", label: "Dark" }
                ]
                current: page.st.theme
                onPicked: page.set("theme", key)
            }

            Label {
                text: "Accent"
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }

            Row {
                width: parent.width
                spacing: Pulse.m

                Repeater {
                    model: Pulse.accents

                    delegate: Column {
                        spacing: Pulse.xs

                        Rectangle {
                            width: units.gu(5)
                            height: units.gu(5)
                            radius: width / 2
                            color: modelData.color
                            border.width: page.st.accent === modelData.key ? units.dp(3) : 0
                            border.color: Pulse.text

                            Glyph {
                                anchors.centerIn: parent
                                visible: page.st.accent === modelData.key
                                name: "check"
                                size: units.gu(2)
                                color: Pulse.dark ? "#07070A" : "#FFFFFF"
                                weight: 2.6
                            }

                            MouseArea {
                                anchors.fill: parent
                                onClicked: page.set("accent", modelData.key)
                            }
                        }

                        Label {
                            width: units.gu(5)
                            horizontalAlignment: Text.AlignHCenter
                            text: modelData.label.split(" ")[modelData.label.split(" ").length - 1]
                            color: page.st.accent === modelData.key ? Pulse.text : Pulse.textDim
                            font.family: Pulse.face
                            font.pixelSize: Pulse.micro
                            elide: Text.ElideRight
                        }
                    }
                }
            }

            Label {
                text: "Units"
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }

            Segmented {
                width: parent.width
                options: [
                    { key: "metric", label: "Metric" },
                    { key: "imperial", label: "Imperial" }
                ]
                current: page.st.units
                onPicked: page.set("units", key)
            }
        }

        // ---- goals -------------------------------------------------------
        SectionTitle { width: parent.width; text: "Goals" }

        Card {
            width: parent.width
            padding: Pulse.m
            spacing: 0

            SettingRow {
                title: "Steps"
                subtitle: "Daily target driving the main ring"
                glyph: "steps"
                Stepper {
                    value: page.st.stepsGoal
                    step: 500
                    minimum: 500
                    maximum: 60000
                    display: Fmt.thousands(page.st.stepsGoal)
                    onValueRequested: page.set("stepsGoal", value)
                }
            }

            SettingRow {
                title: "Sleep"
                subtitle: "Hours and minutes per night"
                glyph: "moon"
                Stepper {
                    value: page.st.sleepGoalMinutes
                    step: 15
                    minimum: 15
                    maximum: 840
                    display: Fmt.durationTrim(page.st.sleepGoalMinutes)
                    onValueRequested: page.set("sleepGoalMinutes", value)
                }
            }

            SettingRow {
                title: "Active calories"
                glyph: "flame"
                Stepper {
                    value: page.st.activeCaloriesGoal
                    step: 25
                    minimum: 25
                    maximum: 5000
                    display: Fmt.thousands(page.st.activeCaloriesGoal)
                    onValueRequested: page.set("activeCaloriesGoal", value)
                }
            }

            SettingRow {
                title: "Distance"
                glyph: "route"
                Stepper {
                    value: page.st.distanceGoalM
                    step: 500
                    minimum: 500
                    maximum: 100000
                    display: Fmt.distance(page.st.distanceGoalM, page.st.units)
                    onValueRequested: page.set("distanceGoalM", value)
                }
            }

            SettingRow {
                title: "Active minutes"
                glyph: "bolt"
                Stepper {
                    value: page.st.activeMinutesGoal
                    step: 5
                    minimum: 5
                    maximum: 600
                    display: page.st.activeMinutesGoal + "m"
                    onValueRequested: page.set("activeMinutesGoal", value)
                }
            }

            SettingRow {
                title: "Intensity minutes"
                subtitle: "Moderate + 2\u00d7 vigorous, per day"
                glyph: "bolt"
                divider: false
                Stepper {
                    value: page.st.intensityGoal
                    step: 5
                    minimum: 5
                    maximum: 300
                    display: page.st.intensityGoal + "m"
                    onValueRequested: page.set("intensityGoal", value)
                }
            }
        }

        // ---- behaviour --------------------------------------------------
        SectionTitle { width: parent.width; text: "Watch behaviour" }

        Card {
            width: parent.width
            padding: Pulse.m
            spacing: 0

            SettingRow {
                title: "Forward notifications"
                subtitle: "Send desktop notifications to the watch"
                glyph: "bell"
                Toggle {
                    checked: page.st.notificationsEnabled
                    onToggled: page.set("notificationsEnabled", value)
                }
            }

            SettingRow {
                title: "Include Waydroid apps"
                subtitle: "Also forward Android notifications from the container"
                glyph: "android"
                Toggle {
                    checked: page.st.notifyWaydroid
                    onToggled: page.set("notifyWaydroid", value)
                }
            }

            SettingRow {
                title: "Weather"
                subtitle: "Answer the watch's weather requests"
                glyph: "drop"
                Toggle {
                    checked: page.st.weatherEnabled
                    onToggled: page.set("weatherEnabled", value)
                }
            }

            SettingRow {
                title: "Sync time"
                subtitle: "Set the watch clock on connect"
                glyph: "timer"
                Toggle {
                    checked: page.st.syncTime
                    onToggled: page.set("syncTime", value)
                }
            }

            SettingRow {
                title: "Keep files on watch"
                subtitle: "Do not delete FIT files after download"
                glyph: "desktop"
                Toggle {
                    checked: page.st.keepFilesOnWatch
                    onToggled: page.set("keepFilesOnWatch", value)
                }
            }

            SettingRow {
                title: "Any goal counts for streaks"
                subtitle: "Otherwise only the step goal keeps a streak alive"
                glyph: "star"
                Toggle {
                    checked: page.st.anyGoalStreak
                    onToggled: page.set("anyGoalStreak", value)
                }
            }

            SettingRow {
                title: "Auto sync"
                subtitle: page.st.autoSyncMinutes > 0
                          ? "Every " + page.st.autoSyncMinutes + " minutes"
                          : "Manual only"
                glyph: "sync"
                divider: false
                Stepper {
                    value: page.st.autoSyncMinutes
                    step: 15
                    minimum: 0
                    maximum: 720
                    display: page.st.autoSyncMinutes > 0 ? page.st.autoSyncMinutes + "m" : "off"
                    onValueRequested: page.set("autoSyncMinutes", value)
                }
            }
        }

        Label {
            width: parent.width
            text: Store.online
                  ? "Connected to pulsed " + Store.daemonVersion + " on 127.0.0.1:21830"
                  : "pulsed is not answering on 127.0.0.1:21830"
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.micro
            wrapMode: Text.WordWrap
        }
    }
}
