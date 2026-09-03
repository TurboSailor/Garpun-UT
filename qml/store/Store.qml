pragma Singleton
import QtQuick 2.12
import Ubuntu.Components 1.3
import "../js/Api.js" as Api
import "../js/I18n.js" as I18n
import "../js/Fmt.js" as Fmt
import "../theme"

// Single source of truth for everything the daemon owns. Pages bind to these
// properties and never talk to the network themselves, so an offline daemon
// degrades to "empty but valid" state in exactly one place.
QtObject {
    id: store

    // ---- daemon ---------------------------------------------------------
    property bool online: false
    property bool everAnswered: false
    property string lastError: ""
    property var status: null
    readonly property var device: status ? status.device : null
    readonly property bool connected: !!(device && device.connected)
    readonly property bool initialized: !!(device && device.initialized)
    readonly property bool syncing: !!(status && status.syncing)
    readonly property var progress: status ? status.progress : null
    readonly property string daemonVersion: status && status.daemonVersion ? status.daemonVersion : ""
    readonly property int battery: device && device.batteryLevel ? device.batteryLevel : -1

    // ---- selection ------------------------------------------------------
    property string date: Fmt.todayIso()
    readonly property bool isToday: date === Fmt.todayIso()

    // ---- payloads (null = never loaded, not "empty") ----------------------
    property var today: null
    property var health: null
    property var sleep: null
    property var workouts: null
    property var devices: null
    property var scan: []
    property var pairing: ({ pending: false })
    property bool scanning: false
    property bool ringing: false

    property bool todayLoading: false
    property bool healthLoading: false
    property bool sleepLoading: false
    property bool workoutsLoading: false

    // The health screen opens on today; 7 and 30 day views are one tap away.
    property int healthDays: 1

    // ---- settings --------------------------------------------------------
    property var settings: ({
        theme: "dark",
        accent: "blue",
        stepsGoal: 10000,
        sleepGoalMinutes: 480,
        activeCaloriesGoal: 350,
        distanceGoalM: 5000,
        activeMinutesGoal: 60,
        intensityGoal: 30,
        units: "metric",
        syncTime: true,
        weatherEnabled: true,
        notificationsEnabled: true,
        notifyWaydroid: true,
        keepFilesOnWatch: false,
        anyGoalStreak: false,
        autoSyncMinutes: 60
    })
    property bool settingsLoaded: false

    // Toast plumbing as plain state: Main watches the counter, so no
    // Connections object is needed anywhere.
    property string toastText: ""
    property int toastSeq: 0
    function toast(message) {
        toastText = message;
        toastSeq = toastSeq + 1;
    }

    // =====================================================================
    // loaders
    // =====================================================================
    function markOnline() {
        online = true;
        everAnswered = true;
        lastError = "";
    }
    function markOffline(msg) {
        online = false;
        lastError = msg;
    }

    function refreshStatus() {
        Api.status(function (r) {
            markOnline();
            status = r;
        }, function (msg) {
            markOffline(msg);
            status = null;
        });
    }

    function loadToday() {
        todayLoading = true;
        Api.today(date, function (r) {
            markOnline();
            today = r;
            todayLoading = false;
        }, function (msg) {
            markOffline(msg);
            todayLoading = false;
        });
    }

    function loadHealth() {
        healthLoading = true;
        Api.health(healthDays, function (r) {
            markOnline();
            health = r && r.metrics ? r.metrics : [];
            healthLoading = false;
        }, function (msg) {
            markOffline(msg);
            healthLoading = false;
        });
    }

    function loadSleep() {
        sleepLoading = true;
        Api.sleep(date, function (r) {
            markOnline();
            sleep = r;
            sleepLoading = false;
        }, function (msg) {
            markOffline(msg);
            sleepLoading = false;
        });
    }

    function loadWorkouts() {
        workoutsLoading = true;
        Api.workouts(50, function (r) {
            markOnline();
            workouts = r || [];
            workoutsLoading = false;
        }, function (msg) {
            markOffline(msg);
            workoutsLoading = false;
        });
    }

    function loadDevices() {
        Api.devices(function (r) {
            markOnline();
            devices = r || [];
        }, function (msg) {
            markOffline(msg);
        });
    }

    function loadSettings() {
        Api.settingsGet(function (r) {
            markOnline();
            if (r) {
                settings = r;
                applyAppearance();
            }
            settingsLoaded = true;
        }, function (msg) {
            markOffline(msg);
            // Keep local defaults so Settings stays usable while offline.
            applyAppearance();
        });
    }

    function applyAppearance() {
        Pulse.mode = settings.theme || "dark";
        Pulse.accentName = settings.accent || "blue";
    }

    // Optimistic local update + PUT of the whole document, as the contract has
    // no partial update verb.
    function setSetting(key, value) {
        var next = {};
        for (var k in settings) next[k] = settings[k];
        next[key] = value;
        settings = next;
        applyAppearance();
        Api.settingsPut(next, function (r) {
            markOnline();
            if (r) {
                settings = r;
                applyAppearance();
            }
        }, function (msg) {
            markOffline(msg);
            toast(I18n.t("toast.settings_save_failed"));
        });
    }

    // =====================================================================
    // actions
    // =====================================================================
    function startScan() {
        scan = [];
        scanning = true;
        Api.scanStart(20000, function () {
            markOnline();
            scanPoll.restart();
            scanStop.restart();
        }, function (msg) {
            markOffline(msg);
            scanning = false;
            toast(I18n.t("toast.scan_failed", [msg]));
        });
    }

    function stopScan() {
        scanning = false;
        scanPoll.stop();
        scanStop.stop();
    }

    function pollScan() {
        Api.scanResults(function (r) {
            markOnline();
            scan = r || [];
        }, function (msg) {
            markOffline(msg);
        });
    }

    function pairDevice(addr) {
        Api.pair(addr, function () {
            markOnline();
            pairingPoll.restart();
        }, function (msg) {
            markOffline(msg);
            toast(I18n.t("toast.pairing_failed", [msg]));
        });
    }

    function pollPairing() {
        Api.pairingState(function (r) {
            markOnline();
            pairing = r || { pending: false };
            if (!pairing.pending) pairingPoll.stop();
        }, function (msg) {
            markOffline(msg);
            pairingPoll.stop();
        });
    }

    function replyPairing(body) {
        Api.pairingReply(body, function () {
            markOnline();
            pairing = { pending: false };
            loadDevices();
        }, function (msg) {
            markOffline(msg);
            toast(I18n.t("toast.pairing_reply_failed"));
        });
    }

    function connectDevice(addr) {
        Api.connect(addr, function () { markOnline(); refreshStatus(); },
                    function (msg) { markOffline(msg); toast(I18n.t("toast.connect_failed")); });
    }
    function disconnectDevice() {
        Api.disconnect(function () { markOnline(); refreshStatus(); },
                       function (msg) { markOffline(msg); toast(I18n.t("toast.disconnect_failed")); });
    }
    function forgetDevice(addr) {
        Api.forget(addr, function () { markOnline(); loadDevices(); refreshStatus(); },
                   function (msg) { markOffline(msg); toast(I18n.t("toast.forget_failed")); });
    }
    function startSync() {
        Api.sync(function () { markOnline(); refreshStatus(); },
                 function (msg) { markOffline(msg); toast(I18n.t("toast.sync_failed")); });
    }
    function findWatch(sec) {
        Api.findWatch(sec, function () {
            markOnline();
            ringing = true;
            ringTimer.interval = sec * 1000;
            ringTimer.restart();
            toast(I18n.t("toast.watch_ringing"));
        }, function (msg) {
            markOffline(msg);
            toast(I18n.t("toast.find_watch_failed"));
        });
    }
    function cancelFindWatch() {
        ringing = false;
        ringTimer.stop();
        Api.findWatchCancel(function () { markOnline(); }, function (msg) { markOffline(msg); });
    }

    function setDate(iso) {
        if (iso === date) return;
        date = iso;
        loadToday();
        loadSleep();
    }

    // =====================================================================
    // event stream
    // =====================================================================
    property var stream: null

    function openStream() {
        if (stream) stream.close();
        stream = new Api.EventStream(onEvent, function (up) {
            if (up) markOnline();
        });
        stream.open();
    }

    function onEvent(ev) {
        markOnline();
        var kind = ev && ev.kind ? ev.kind : "";
        var data = ev ? ev.data : null;
        switch (kind) {
        case "scan_result":
            if (data) mergeScan(data);
            return;
        case "pairing_request":
            pairing = data || { pending: false };
            return;
        case "settings_changed":
            if (data) { settings = data; applyAppearance(); }
            else loadSettings();
            return;
        case "sync_started":
        case "sync_progress":
        case "battery":
        case "device_info":
        case "initialized":
        case "capabilities":
        case "disconnected":
            refreshStatus();
            return;
        case "sync_finished":
            refreshStatus();
            reloadDebounce.restart();
            return;
        case "log":
            return;
        }
        refreshStatus();
    }

    function mergeScan(entry) {
        var list = scan.slice();
        for (var i = 0; i < list.length; i++) {
            if (list[i].address === entry.address) {
                list[i] = entry;
                scan = list;
                return;
            }
        }
        list.push(entry);
        scan = list;
    }

    function start() {
        loadSettings();
        refreshStatus();
        loadDevices();
        loadToday();
        loadHealth();
        loadSleep();
        loadWorkouts();
        openStream();
        statusPoll.start();
    }

    // =====================================================================
    // timers
    // =====================================================================
    // Polling is the fallback path: the SSE stream carries the fast updates,
    // but a daemon that is not running yet must still be picked up.
    property Timer statusPoll: Timer {
        interval: 5000
        repeat: true
        onTriggered: {
            store.refreshStatus();
            if (!store.stream || !store.stream.connected())
                store.openStream();
        }
    }

    property Timer scanPoll: Timer {
        interval: 1200
        repeat: true
        onTriggered: store.pollScan()
    }

    property Timer scanStop: Timer {
        interval: 21000
        repeat: false
        onTriggered: store.stopScan()
    }

    property Timer pairingPoll: Timer {
        interval: 800
        repeat: true
        onTriggered: store.pollPairing()
    }

    // The daemon stops the alarm on its own after `seconds`; mirror that here
    // so the button goes back to "Find my watch" without a round trip.
    property Timer ringTimer: Timer {
        interval: 30000
        repeat: false
        onTriggered: store.ringing = false
    }

    property Timer reloadDebounce: Timer {
        interval: 900
        repeat: false
        onTriggered: {
            store.loadToday();
            store.loadHealth();
            store.loadSleep();
            store.loadWorkouts();
        }
    }
}
