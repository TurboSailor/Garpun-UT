import QtQuick 2.12
import QtQuick.Shapes 1.12
import Ubuntu.Components 1.3
import "../components"
import "../theme"
import "../js/I18n.js" as I18n

// Renders every glyph in the set to a grid and saves it as a PNG so the icon
// sheet can be eyeballed without a phone in hand:
//   QT_QPA_PLATFORM=minimal qmlscene dev/shot.qml
Rectangle {
    id: sheet
    width: 900
    height: 700
    color: "#07070A"

    readonly property var names: [
        "steps", "flame", "route", "bolt", "heart", "pulse", "battery", "moon",
        "wave", "drop", "gauge", "bell", "watch", "sliders", "chevron", "back",
        "sync", "search", "check", "close", "plus", "minus", "timer", "phone",
        "android", "desktop", "trash", "bluetooth", "star", "mountain",
        "run", "bike", "swim", "walk", "hike", "strength", "yoga", "cardio",
        "tennis", "basketball", "soccer", "row", "ski", "paddle", "treadmill",
        "multisport", "calendar", "clock", "info", "settings", "phone_off",
        "music", "weather", "map", "lungs", "sleep_score", "trend_up", "trend_down"
    ]

    Grid {
        anchors.centerIn: parent
        columns: 10
        spacing: 26

        Repeater {
            model: sheet.names
            delegate: Column {
                spacing: 6
                width: 62

                Glyph {
                    anchors.horizontalCenter: parent.horizontalCenter
                    name: modelData
                    size: 34
                    color: "#2BD8FF"
                }
                Text {
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: 62
                    horizontalAlignment: Text.AlignHCenter
                    text: modelData
                    color: "#8A8A93"
                    font.pixelSize: 9
                    elide: Text.ElideRight
                }
            }
        }
    }

    Component.onCompleted: {
        var missing = [];
        for (var i = 0; i < names.length; i++) {
            var probe = glyphProbe.createObject(sheet, { name: names[i] });
            if (probe.path.length === 0) missing.push(names[i]);
            probe.destroy();
        }
        console.log("MISSING (" + missing.length + "): " + missing.join(", "));
        sheet.grabToImage(function (result) {
            result.saveToFile("/tmp/glyphs.png");
            console.log("saved /tmp/glyphs.png");
            Qt.quit();
        });
    }

    Component {
        id: glyphProbe
        Glyph {}
    }
}
