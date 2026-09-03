import QtQuick 2.12
import UserMetrics 0.1
import "../js/I18n.js" as I18n

// The only file in the tree that touches libusermetrics. LockScreenMetrics
// pulls it in through a Loader, so the `import UserMetrics 0.1` above is
// allowed to fail on a system without the plugin: the failure is confined to
// one component instead of aborting the whole QML tree.
//
// libusermetrics keeps one counter per `name` and renders `format` with %1
// replaced by the pushed value. `emptyFormat` is what the lock screen shows
// until the first update of the day, so a metric we have no data for is left
// alone rather than pushed as zero.
QtObject {
    id: sink

    // Must match applicationName / the click package name: the shell groups
    // the infographic entries by it.
    readonly property string domain: "pulse.turbosailor"

    readonly property Metric stepsMetric: Metric {
        name: "pulse-steps"
        domain: sink.domain
        format: I18n.t("lock.steps")
        emptyFormat: I18n.t("lock.steps_empty")
        minimum: 0
    }

    readonly property Metric bodyMetric: Metric {
        name: "pulse-bodybattery"
        domain: sink.domain
        format: I18n.t("lock.body_battery")
        emptyFormat: I18n.t("lock.body_battery_empty")
        minimum: 0
        maximum: 100
    }

    readonly property Metric batteryMetric: Metric {
        name: "pulse-battery"
        domain: sink.domain
        format: I18n.t("lock.watch_battery")
        emptyFormat: I18n.t("lock.watch_battery_empty")
        minimum: 0
        maximum: 100
    }

    // Keys are the ones LockScreenMetrics throttles on; an unknown key is a
    // programming error, not a runtime condition, so it fails loudly.
    function publish(key, value) {
        switch (key) {
        case "steps":   stepsMetric.update(value);   return;
        case "body":    bodyMetric.update(value);    return;
        case "battery": batteryMetric.update(value); return;
        }
        console.log("UserMetricsSink: unknown metric key " + key);
    }
}
