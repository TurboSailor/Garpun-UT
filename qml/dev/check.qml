import QtQuick 2.12
import Ubuntu.Components 1.3

// Compile-only validation harness. Every QML file in the app is turned into a
// Component, which makes the engine parse it and resolve every type and
// property without rendering anything. Run on the device:
//
//   QT_QPA_PLATFORM=offscreen qmlscene dev/check.qml
//
// It prints one line per file and exits with a FAILED summary if any file did
// not compile.
Item {
    id: root
    width: 1
    height: 1

    // The two singletons are deliberately absent: QQmlComponent refuses
    // `pragma Singleton` files, and compiling one poisons the type cache for
    // every later import of it. They are exercised through every file below.
    readonly property var files: [
        "../components/Card.qml",
        "../components/DateNav.qml",
        "../components/DeltaChip.qml",
        "../components/DeviceRow.qml",
        "../components/EmptyState.qml",
        "../components/Glyph.qml",
        "../components/HealthCard.qml",
        "../components/Hypnogram.qml",
        "../components/MetricTile.qml",
        "../components/NavBar.qml",
        "../components/PageHead.qml",
        "../components/PairingSheet.qml",
        "../components/PillButton.qml",
        "../components/ProgressLine.qml",
        "../components/RingGauge.qml",
        "../components/RouteMap.qml",
        "../components/ScanRow.qml",
        "../components/Screen.qml",
        "../components/SectionTitle.qml",
        "../components/Segmented.qml",
        "../components/SeriesChart.qml",
        "../components/SettingRow.qml",
        "../components/Skeleton.qml",
        "../components/Sparkline.qml",
        "../components/StageBar.qml",
        "../components/StatusStrip.qml",
        "../components/Stepper.qml",
        "../components/Toast.qml",
        "../components/Toggle.qml",
        "../components/TrendBars.qml",
        "../components/WorkoutRow.qml",
        "../pages/DevicePage.qml",
        "../pages/FitnessPage.qml",
        "../pages/HealthPage.qml",
        "../pages/MetricDetailPage.qml",
        "../pages/NotificationsPage.qml",
        "../pages/SleepPage.qml",
        "../pages/TodayPage.qml",
        "../pages/WorkoutDetailPage.qml",
        "../Main.qml"
    ]

    Component.onCompleted: {
        var failed = 0;
        for (var i = 0; i < files.length; i++) {
            var c = Qt.createComponent(Qt.resolvedUrl(files[i]), Component.PreferSynchronous);
            if (c.status === Component.Error) {
                failed++;
                console.log("FAIL " + files[i] + "\n" + c.errorString());
            } else {
                console.log("ok   " + files[i]);
            }
            c.destroy();
        }
        console.log(failed === 0
                    ? "ALL OK (" + files.length + " files)"
                    : "FAILED " + failed + " of " + files.length);
        Qt.quit();
    }
}
