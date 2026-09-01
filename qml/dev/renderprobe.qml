import QtQuick 2.12
import QtQuick.Shapes 1.12

// Renders the same mark several ways to find out which primitive this device
// actually draws, and whether the Scale transform in Glyph.qml is the problem:
//   1  Shape filled by anchors + Scale   (exactly what Glyph.qml does today)
//   2  Shape sized 24x24 + Scale         (the suspected fix)
//   3  Shape drawn at final size, no transform
//   4  Canvas 2D stroke                  (fallback if Shape is broken here)
Rectangle {
    width: 360
    height: 320
    color: "#07070A"

    readonly property string tick: "M4.2 12.6l5.2 5.2L20 6.6"
    readonly property color ink: "#2BD8FF"
    readonly property int box: 48

    Column {
        anchors.centerIn: parent
        spacing: 14

        Row {
            spacing: 14
            Text { text: "1 fill+Scale"; color: "white"; font.pixelSize: 12; width: 120 }
            Rectangle {
                width: box; height: box; color: "#17171F"
                Shape {
                    anchors.fill: parent
                    antialiasing: true
                    transform: Scale { xScale: box / 24; yScale: box / 24 }
                    ShapePath {
                        strokeColor: ink; strokeWidth: 1.9; fillColor: "transparent"
                        capStyle: ShapePath.RoundCap
                        PathSvg { path: tick }
                    }
                }
            }
        }

        Row {
            spacing: 14
            Text { text: "2 24x24+Scale"; color: "white"; font.pixelSize: 12; width: 120 }
            Rectangle {
                width: box; height: box; color: "#17171F"
                Shape {
                    width: 24; height: 24
                    antialiasing: true
                    transform: Scale { xScale: box / 24; yScale: box / 24 }
                    ShapePath {
                        strokeColor: ink; strokeWidth: 1.9; fillColor: "transparent"
                        capStyle: ShapePath.RoundCap
                        PathSvg { path: tick }
                    }
                }
            }
        }

        Row {
            spacing: 14
            Text { text: "3 no transform"; color: "white"; font.pixelSize: 12; width: 120 }
            Rectangle {
                width: box; height: box; color: "#17171F"
                Shape {
                    anchors.fill: parent
                    antialiasing: true
                    ShapePath {
                        strokeColor: ink; strokeWidth: 4; fillColor: "transparent"
                        capStyle: ShapePath.RoundCap
                        PathSvg { path: "M8 25l10 10L40 13" }
                    }
                }
            }
        }

        Row {
            spacing: 14
            Text { text: "4 Canvas 2D"; color: "white"; font.pixelSize: 12; width: 120 }
            Rectangle {
                width: box; height: box; color: "#17171F"
                Canvas {
                    anchors.fill: parent
                    onPaint: {
                        var c = getContext("2d");
                        c.reset();
                        c.strokeStyle = "#2BD8FF";
                        c.lineWidth = 4;
                        c.lineCap = "round";
                        c.lineJoin = "round";
                        c.beginPath();
                        c.moveTo(8, 25); c.lineTo(18, 35); c.lineTo(40, 13);
                        c.stroke();
                    }
                }
            }
        }
    }
}
