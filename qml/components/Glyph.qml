import QtQuick 2.12
import QtQuick.Shapes 1.12
import Ubuntu.Components 1.3
import "../theme"

// Hand-drawn stroke marks on a 24x24 grid. Deliberately one weight, one
// corner treatment, no filled badges behind them: the icon set is part of the
// typography, not decoration.
Item {
    id: root

    property string name: ""
    property real size: units.gu(2.5)
    property color color: Pulse.text
    property real weight: 1.9

    width: size
    height: size

    readonly property string path: {
        switch (name) {
        case "steps":
            return "M3 19h4v-4h4v-4h4V7h5";
        case "flame":
            return "M12 2.5c1.6 3.6 4.6 5 4.6 8.6a4.6 4.6 0 0 1-9.2 0c0-2 1-3.1 2.1-4.3 0 1.6.8 2.4 1.6 2.4 1 -1.7 .9-4.2 .9-6.7z";
        case "route":
            return "M5 19c4.5 0 4-6 8.5-6S18 7 19 6.5 M5 19.6a1.6 1.6 0 1 0 0-3.2 1.6 1.6 0 0 0 0 3.2z M19 8.1a1.6 1.6 0 1 0 0-3.2 1.6 1.6 0 0 0 0 3.2z";
        case "bolt":
            return "M13.5 2.5L5.5 13.5h5.2L10 21.5l8.4-11.4h-5.4z";
        case "heart":
            return "M12 20.2C10.4 19 4.8 15.2 4.8 10.9A3.9 3.9 0 0 1 12 8.6a3.9 3.9 0 0 1 7.2 2.3c0 4.3-5.6 8.1-7.2 9.3z";
        case "pulse":
            return "M2.5 12h4.2l2-5.2 3.1 10.4 2.1-5.2h7.6";
        case "battery":
            return "M3.5 8.2h13.2v7.6H3.5zM18.6 10.9h1.9v2.2h-1.9z";
        case "moon":
            return "M20.2 14.8A8.6 8.6 0 0 1 9.2 3.8 8.6 8.6 0 1 0 20.2 14.8z";
        case "wave":
            return "M2.5 12.5h3.1l2-4.8 3 9.4 2.6-8 2.2 5 1.6-1.6h4.5";
        case "drop":
            return "M12 3.2s6 6.6 6 10.1a6 6 0 0 1-12 0c0-3.5 6-10.1 6-10.1z";
        case "gauge":
            return "M4 17.5a8 8 0 1 1 16 0 M12 17.2l4.2-5.4";
        case "bell":
            return "M6.2 16.4v-5a5.8 5.8 0 0 1 11.6 0v5l1.8 2H4.4zM10 21.2h4";
        case "watch":
            return "M8.4 6.6V2.8h7.2v3.8M8.4 17.4v3.8h7.2v-3.8M6 12a6 6 0 1 1 12 0 6 6 0 0 1-12 0z";
        case "sliders":
            return "M4 7.2h9M17.4 7.2H20M4 16.8h4.4M12.8 16.8H20M15.2 4.4v5.6M10.6 14v5.6";
        case "chevron":
            return "M9.4 5.2l6.8 6.8-6.8 6.8";
        case "back":
            return "M14.6 5.2L7.8 12l6.8 6.8";
        case "sync":
            return "M20 12a8 8 0 1 1-2.4-5.7M20.2 3.4v5.2h-5.2";
        case "search":
            return "M11 4.2a6.8 6.8 0 1 0 0 13.6 6.8 6.8 0 0 0 0-13.6zM16 16.2l4.6 4.6";
        case "check":
            return "M4.2 12.6l5.2 5.2L20 6.6";
        case "close":
            return "M6 6l12 12M18 6L6 18";
        case "plus":
            return "M12 4.8v14.4M4.8 12h14.4";
        case "minus":
            return "M4.8 12h14.4";
        case "timer":
            return "M12 21a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM12 13.2V9.4M9 2.4h6M18.6 5.6l1.6-1.6";
        case "phone":
            return "M6.2 3.4l2.9 4.9-2 2a12.4 12.4 0 0 0 5.2 5.2l2-2 4.9 2.9-2 3c-6.2 1-13.9-6.7-12.9-12.9z";
        case "android":
            return "M6.2 10.4h11.6v7a2 2 0 0 1-2 2h-7.6a2 2 0 0 1-2-2zM9.2 10.4a2.8 2.8 0 0 1 5.6 0M8.6 6.6L7.2 4.4M15.4 6.6L16.8 4.4";
        case "desktop":
            return "M3.2 5h17.6v10.6H3.2zM9 20h6M12 15.6V20";
        case "trash":
            return "M4.8 7h14.4M9.2 7V3.8h5.6V7M6.8 7l1 13.2h8.4L17 7";
        case "bluetooth":
            return "M8 7.6L16 16.4 12 20V4l4 3.6L8 16.4";
        case "star":
            return "M12 3.4l2.6 5.6 6 .8-4.4 4.2 1.1 6-5.3-2.9-5.3 2.9 1.1-6L3.4 9.8l6-.8z";
        case "mountain":
            return "M2.6 19h18.8L14 6.4l-3.4 5.8-2.1-2.4z";
        }
        return "";
    }

    Shape {
        anchors.fill: parent
        antialiasing: true
        visible: root.path.length > 0
        transform: Scale { xScale: root.size / 24; yScale: root.size / 24 }

        ShapePath {
            strokeColor: root.color
            strokeWidth: root.weight
            fillColor: "transparent"
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            PathSvg { path: root.path }
        }
    }
}
