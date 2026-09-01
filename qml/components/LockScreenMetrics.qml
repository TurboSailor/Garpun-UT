import QtQuick 2.12
import "../store"

// Feeds the Lomiri lock screen infographic (libusermetrics) with the three
// figures worth glancing at without unlocking: today's steps, Body Battery and
// the watch's own charge. Invisible and interaction-free — it is a data sink,
// not a widget.
//
// Two rules govern what leaves the app:
//   * only real readings are published, so a metric we know nothing about
//     keeps showing its `emptyFormat` instead of a misleading zero;
//   * at most one write per metric per minute, with the newest value flushed
//     once the minute is up, because every update is a system D-Bus round trip.
Item {
    id: root

    visible: false
    width: 0
    height: 0

    // -1 means "nothing to report"; the daemon uses the same convention.
    readonly property int stepsValue: (Store.isToday && Store.today && Store.today.steps > 0)
                                      ? Store.today.steps : -1

    readonly property int bodyValue: {
        if (!Store.isToday || !Store.today || !Store.today.bodyEnergy) return -1;
        var v = Store.today.bodyEnergy.latest;
        return v > 0 ? v : -1;
    }

    readonly property int watchBatteryValue: (Store.connected && Store.battery > 0) ? Store.battery : -1

    readonly property int minIntervalMs: 60000

    // Plain JS bookkeeping: key -> { value, at } for what the service already
    // holds, key -> value for what is waiting out the rate limit. Nothing
    // binds to either, so mutating them in place is fine.
    property var published: ({})
    property var waiting: ({})

    // Set once the UserMetrics plugin turns out to be missing; from then on
    // every offer is a no-op instead of a retry storm.
    property bool unavailable: false

    onStepsValueChanged: offer("steps", stepsValue)
    onBodyValueChanged: offer("body", bodyValue)
    onWatchBatteryValueChanged: offer("battery", watchBatteryValue)

    function offer(key, value) {
        if (unavailable) return;
        if (value < 0) { delete waiting[key]; return; }

        var last = published[key];
        if (last && last.value === value) { delete waiting[key]; return; }

        if (!last || Date.now() - last.at >= minIntervalMs) {
            send(key, value);
            return;
        }

        waiting[key] = value;
        if (!flush.running) flush.start();
    }

    function send(key, value) {
        delete waiting[key];

        var sink = resolveSink();
        if (!sink) return;

        try {
            sink.publish(key, value);
        } catch (e) {
            console.log("Pulse: lock screen metric '" + key + "' rejected: " + e);
            return;
        }
        published[key] = { value: value, at: Date.now() };
    }

    // Loaded on first real reading rather than at startup: by then the locale
    // has been detected, so the metric formats are registered in the right
    // language, and an app that never sees data never touches the service.
    function resolveSink() {
        if (unavailable) return null;
        if (loader.item) return loader.item;

        loader.source = Qt.resolvedUrl("UserMetricsSink.qml");
        if (loader.status === Loader.Error || !loader.item) {
            unavailable = true;
            waiting = ({});
            flush.stop();
            console.log("Pulse: UserMetrics plugin unavailable, lock screen metrics disabled");
            return null;
        }
        return loader.item;
    }

    Loader {
        id: loader
        asynchronous: false
        active: true
    }

    Timer {
        id: flush
        interval: 5000
        repeat: true
        onTriggered: {
            var keys = Object.keys(root.waiting);
            var stillWaiting = 0;
            for (var i = 0; i < keys.length; i++) {
                var key = keys[i];
                var last = root.published[key];
                if (!last || Date.now() - last.at >= root.minIntervalMs)
                    root.send(key, root.waiting[key]);
                else
                    stillWaiting++;
            }
            if (stillWaiting === 0) stop();
        }
    }
}
