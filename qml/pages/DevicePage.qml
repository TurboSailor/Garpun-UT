import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt
import "../js/I18n.js" as I18n

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
            title: I18n.t("device.title")
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
                text: Store.connected ? I18n.t("action.disconnect") : I18n.t("action.connect")
                glyph: "bluetooth"
                kind: Store.connected ? "ghost" : "primary"
                enabledLook: Store.online && !!Store.device
                onClicked: {
                    if (Store.connected) Store.disconnectDevice();
                    else if (Store.device) Store.connectDevice(Store.device.address);
                }
            }

            PillButton {
                text: Store.syncing ? I18n.t("action.syncing") : I18n.t("action.sync")
                glyph: "sync"
                busy: Store.syncing
                enabledLook: Store.connected && !Store.syncing
                onClicked: Store.startSync()
            }

            PillButton {
                text: Store.ringing ? I18n.t("action.stop_ringing") : I18n.t("action.find_watch")
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
                text: I18n.t("device.transferring")
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
                    return I18n.t("device.files_progress", [
                        p.fileIndex + 1, Fmt.thousands(p.received), Fmt.thousands(p.total), Fmt.thousands(p.remaining)
                    ]);
                }
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }
        }

        // ---- paired ----------------------------------------------------
        SectionTitle { width: parent.width; text: I18n.t("device.paired_section"); glyph: "watch" }

        Card {
            width: parent.width
            padding: page.paired.length > 0 ? Pulse.s : Pulse.l
            spacing: 0

            EmptyState {
                width: parent.width
                visible: page.paired.length === 0
                glyph: "watch"
                title: Store.online ? I18n.t("device.no_watch_paired") : I18n.t("status.daemon_offline")
                hint: Store.online
                      ? I18n.t("device.pair_hint")
                      : I18n.t("device.daemon_offline_hint")
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
            text: I18n.t("device.nearby_section")
            glyph: "bluetooth"
            action: Store.scanning ? I18n.t("action.stop") : I18n.t("action.scan")
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
                title: Store.scanning ? I18n.t("device.scanning") : I18n.t("device.nothing_found")
                hint: Store.scanning
                      ? I18n.t("device.keep_awake_hint")
                      : I18n.t("device.scan_hint")
                action: Store.scanning || !Store.online ? "" : I18n.t("action.scan")
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
        SectionTitle { width: parent.width; text: I18n.t("device.appearance_section"); glyph: "sliders" }

        Card {
            width: parent.width

            Label {
                text: I18n.t("device.theme")
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }

            Segmented {
                width: parent.width
                options: [
                    { key: "system", label: I18n.t("device.theme_system") },
                    { key: "light", label: I18n.t("device.theme_light") },
                    { key: "dark", label: I18n.t("device.theme_dark") }
                ]
                current: page.st.theme
                onPicked: page.set("theme", key)
            }

            Label {
                text: I18n.t("device.accent")
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
                text: I18n.t("device.units")
                color: Pulse.textDim
                font.family: Pulse.face
                font.pixelSize: Pulse.caption
            }

            Segmented {
                width: parent.width
                options: [
                    { key: "metric", label: I18n.t("device.units_metric") },
                    { key: "imperial", label: I18n.t("device.units_imperial") }
                ]
                current: page.st.units
                onPicked: page.set("units", key)
            }
        }

        // ---- goals -------------------------------------------------------
        SectionTitle { width: parent.width; text: I18n.t("device.goals_section"); glyph: "star" }

        Card {
            width: parent.width
            padding: Pulse.m
            spacing: 0

            SettingRow {
                title: I18n.t("device.goal_steps")
                subtitle: I18n.isRu() ? "Дневная цель для кольца активности" : "Daily target driving the main ring"
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
                title: I18n.t("device.goal_sleep")
                subtitle: I18n.isRu() ? "Количество минут сна за ночь" : "Hours and minutes per night"
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
                title: I18n.t("device.goal_calories")
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
                title: I18n.t("device.goal_distance")
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
                title: I18n.t("device.goal_active_mins")
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
                title: I18n.t("device.goal_intensity")
                subtitle: I18n.isRu() ? "Умеренная + 2× высокая нагрузка" : "Moderate + 2\u00d7 vigorous, per day"
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
        SectionTitle { width: parent.width; text: I18n.t("device.integrations_section"); glyph: "settings" }

        Card {
            width: parent.width
            padding: Pulse.m
            spacing: 0

            SettingRow {
                title: I18n.t("device.notifications_title")
                subtitle: I18n.t("device.notifications_sub")
                glyph: "bell"
                Toggle {
                    checked: page.st.notificationsEnabled
                    onToggled: page.set("notificationsEnabled", value)
                }
            }

            SettingRow {
                title: I18n.t("device.waydroid_title")
                subtitle: I18n.t("device.waydroid_sub")
                glyph: "android"
                Toggle {
                    checked: page.st.notifyWaydroid
                    onToggled: page.set("notifyWaydroid", value)
                }
            }

            SettingRow {
                title: I18n.t("device.weather_title")
                subtitle: I18n.t("device.weather_sub")
                glyph: "drop"
                Toggle {
                    checked: page.st.weatherEnabled
                    onToggled: page.set("weatherEnabled", value)
                }
            }

            SettingRow {
                title: I18n.t("device.sync_time_title")
                subtitle: I18n.t("device.sync_time_sub")
                glyph: "timer"
                Toggle {
                    checked: page.st.syncTime
                    onToggled: page.set("syncTime", value)
                }
            }

            SettingRow {
                title: I18n.t("device.keep_files_title")
                subtitle: I18n.t("device.keep_files_sub")
                glyph: "desktop"
                Toggle {
                    checked: page.st.keepFilesOnWatch
                    onToggled: page.set("keepFilesOnWatch", value)
                }
            }

            SettingRow {
                title: I18n.isRu() ? "Любая цель продлевает серию" : "Any goal counts for streaks"
                subtitle: I18n.isRu() ? "Иначе серию поддерживает только цель по шагам" : "Otherwise only the step goal keeps a streak alive"
                glyph: "star"
                Toggle {
                    checked: page.st.anyGoalStreak
                    onToggled: page.set("anyGoalStreak", value)
                }
            }

            SettingRow {
                title: I18n.isRu() ? "Автосинхронизация" : "Auto sync"
                subtitle: page.st.autoSyncMinutes > 0
                          ? (I18n.isRu() ? ("Каждые " + page.st.autoSyncMinutes + " мин") : ("Every " + page.st.autoSyncMinutes + " minutes"))
                          : (I18n.isRu() ? "Только вручную" : "Manual only")
                glyph: "sync"
                divider: false
                Stepper {
                    value: page.st.autoSyncMinutes
                    step: 15
                    minimum: 0
                    maximum: 720
                    display: page.st.autoSyncMinutes > 0 ? (page.st.autoSyncMinutes + (I18n.isRu() ? " мин" : "m")) : (I18n.isRu() ? "выкл" : "off")
                    onValueRequested: page.set("autoSyncMinutes", value)
                }
            }
        }

        Label {
            width: parent.width
            text: Store.online
                  ? I18n.t("device.footer_online", [Store.daemonVersion])
                  : I18n.t("device.footer_offline")
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.micro
            wrapMode: Text.WordWrap
        }
    }
}
