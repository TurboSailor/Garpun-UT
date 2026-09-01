import QtQuick 2.12
import Ubuntu.Components 1.3
import "theme"
import "store"
import "components"
import "pages"
import "js/I18n.js" as I18n

MainView {
    id: app

    applicationName: "cc.zachy.pulse"
    objectName: "pulseMain"
    anchorToKeyboard: true
    backgroundColor: Pulse.bg

    width: units.gu(45)
    height: units.gu(80)

    // Lomiri ships a dark and a light Suru theme; "system" follows whichever
    // is active instead of guessing.
    // `theme` is a UITK StyledItem property; probing it defensively keeps the
    // app silent if it is ever absent instead of logging a ReferenceError.
    readonly property bool systemDark: {
        try {
            return ("" + theme.name).toLowerCase().indexOf("dark") >= 0;
        } catch (e) {
            return true;
        }
    }
    onSystemDarkChanged: Pulse.systemDark = systemDark

    readonly property int toastSeq: Store.toastSeq
    onToastSeqChanged: toast.show(Store.toastText)

    Component.onCompleted: {
        I18n.detect();
        Pulse.systemDark = systemDark;
        Store.start();
    }

    readonly property var tabs: [
        { key: "today",   label: I18n.t("tab.today"),   glyph: "steps" },
        { key: "health",  label: I18n.t("tab.health"),  glyph: "pulse" },
        { key: "sleep",   label: I18n.t("tab.sleep"),   glyph: "moon" },
        { key: "fitness", label: I18n.t("tab.fitness"), glyph: "timer" },
        { key: "device",  label: I18n.t("tab.device"),  glyph: "watch" }
    ]

    property int tab: 0
    // Pages stay alive after their first visit so scroll position and chart
    // state survive tab switches; unvisited tabs cost nothing.
    property var visited: [true, false, false, false, false]

    property string overlay: ""
    property var overlayArg: null

    function selectTab(key) {
        for (var i = 0; i < tabs.length; i++) {
            if (tabs[i].key === key) { showTab(i); return; }
        }
    }

    function showTab(i) {
        if (i === tab) return;
        var v = visited.slice();
        v[i] = true;
        visited = v;
        tab = i;
    }

    function push(name, arg) {
        overlayArg = arg;
        overlay = name;
    }

    function pop() {
        overlay = "";
    }

    Rectangle {
        anchors.fill: parent
        color: Pulse.bg
    }

    // ---- tabs -------------------------------------------------------------
    Item {
        id: pageHost
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: nav.top

        Repeater {
            model: app.tabs.length

            delegate: Loader {
                anchors.fill: parent
                active: app.visited[index]
                visible: opacity > 0
                opacity: app.tab === index ? 1 : 0

                transform: Translate {
                    id: slide
                    y: app.tab === index ? 0 : units.gu(1.5)
                    Behavior on y { NumberAnimation { duration: Pulse.med; easing.type: Easing.OutQuart } }
                }

                Behavior on opacity { NumberAnimation { duration: Pulse.med; easing.type: Easing.OutQuart } }

                sourceComponent: {
                    switch (index) {
                    case 0: return todayComponent;
                    case 1: return healthComponent;
                    case 2: return sleepComponent;
                    case 3: return fitnessComponent;
                    }
                    return deviceComponent;
                }
            }
        }
    }

    Component {
        id: todayComponent
        TodayPage {
            onOpenNotifications: app.push("notifications", null)
            onOpenWorkout: app.push("workout", workout)
            onOpenTab: app.selectTab(key)
        }
    }

    Component {
        id: healthComponent
        HealthPage {
            onOpenMetric: app.push("metric", metric)
        }
    }

    Component {
        id: sleepComponent
        SleepPage {}
    }

    Component {
        id: fitnessComponent
        FitnessPage {
            onOpenWorkout: app.push("workout", workout)
        }
    }

    Component {
        id: deviceComponent
        DevicePage {
            onOpenNotifications: app.push("notifications", null)
        }
    }

    // ---- nav --------------------------------------------------------------
    NavBar {
        id: nav
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        tabs: app.tabs
        current: app.tab
        onPicked: app.showTab(index)
    }

    // ---- detail overlay ---------------------------------------------------
    Item {
        id: overlayHost
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        width: parent.width
        x: app.overlay.length > 0 ? 0 : parent.width
        visible: x < parent.width

        Behavior on x { NumberAnimation { duration: Pulse.med; easing.type: Easing.OutQuart } }

        Rectangle {
            anchors.fill: parent
            color: Pulse.bg
            MouseArea { anchors.fill: parent }
        }

        Loader {
            anchors.fill: parent
            active: app.overlay.length > 0
            sourceComponent: {
                if (app.overlay === "notifications") return notificationsComponent;
                if (app.overlay === "workout") return workoutComponent;
                if (app.overlay === "metric") return metricComponent;
                return null;
            }
        }
    }

    Component {
        id: notificationsComponent
        NotificationsPage { onBack: app.pop() }
    }

    Component {
        id: workoutComponent
        WorkoutDetailPage {
            stub: app.overlayArg
            onBack: app.pop()
        }
    }

    Component {
        id: metricComponent
        MetricDetailPage {
            metric: app.overlayArg
            onBack: app.pop()
        }
    }

    // ---- global surfaces ---------------------------------------------------
    PairingSheet {
        anchors.fill: parent
    }

    // Feeds the lock screen infographic; no visual footprint.
    LockScreenMetrics {}

    Toast {
        id: toast
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: nav.top
        anchors.bottomMargin: Pulse.l
    }
}
