import QtQuick 2.12
import QtQuick.Shapes 1.12
import Ubuntu.Components 1.3

// Inside MainView the QtQuick.Shapes path draws nothing, although the very
// same code renders standalone. This probe puts the candidates side by side
// in the real environment so the replacement is chosen on evidence:
//   1 Shape        what Glyph.qml uses today
//   2 Canvas 2D    manual strokes
//   3 Image + SVG  the existing path strings, rendered by the svg plugin
MainView {
    id: app
    applicationName: "cc.zachy.pulse"
    width: units.gu(45)
    height: units.gu(80)

    readonly property string tick: "M4.2 12.6l5.2 5.2L20 6.6"
    readonly property string ink: "#2BD8FF"

    Rectangle {
        anchors.fill: parent
        color: "#07070A"

        Column {
            anchors.centerIn: parent
            spacing: units.gu(3)

            Row {
                spacing: units.gu(2)
                Text { text: "1 Shape"; color: "white"; font.pixelSize: 16; width: units.gu(12) }
                Rectangle {
                    width: units.gu(6); height: units.gu(6); color: "#17171F"
                    Shape {
                        anchors.fill: parent
                        antialiasing: true
                        transform: Scale { xScale: units.gu(6) / 24; yScale: units.gu(6) / 24 }
                        ShapePath {
                            strokeColor: app.ink; strokeWidth: 1.9; fillColor: "transparent"
                            capStyle: ShapePath.RoundCap
                            PathSvg { path: app.tick }
                        }
                    }
                }
            }

            Row {
                spacing: units.gu(2)
                Text { text: "2 Canvas"; color: "white"; font.pixelSize: 16; width: units.gu(12) }
                Rectangle {
                    width: units.gu(6); height: units.gu(6); color: "#17171F"
                    Canvas {
                        anchors.fill: parent
                        onPaint: {
                            var k = width / 24;
                            var c = getContext("2d");
                            c.reset();
                            c.strokeStyle = app.ink;
                            c.lineWidth = 1.9 * k;
                            c.lineCap = "round";
                            c.lineJoin = "round";
                            c.beginPath();
                            c.moveTo(4.2 * k, 12.6 * k);
                            c.lineTo(9.4 * k, 17.8 * k);
                            c.lineTo(20 * k, 6.6 * k);
                            c.stroke();
                        }
                    }
                }
            }

            Row {
                spacing: units.gu(2)
                Text { text: "3 Image+SVG"; color: "white"; font.pixelSize: 16; width: units.gu(12) }
                Rectangle {
                    width: units.gu(6); height: units.gu(6); color: "#17171F"
                    Image {
                        anchors.fill: parent
                        smooth: true
                        source: "data:image/svg+xml;utf8," +
                                '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">' +
                                '<path d="' + app.tick + '" fill="none" stroke="' + app.ink +
                                '" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>'
                        onStatusChanged: if (status === Image.Error) console.log("PROBE svg image FAILED")
                    }
                }
            }
        }
    }
}
