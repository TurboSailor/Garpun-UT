import QtQuick 2.12
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
        // ---- sports ----------------------------------------------------
        case "run":
            return "M15 5a1.8 1.8 0 1 0 0-3.6 1.8 1.8 0 0 0 0 3.6z M7 21.6l3.4-5.8-2.4-3 1.8-4.6 4.2-1.4 2.4 3 3.2 1.1 M10.4 15.8l-1.3 2.7-3.9.5";
        case "bike":
            return "M5.6 19.6a3.6 3.6 0 1 0 0-7.2 3.6 3.6 0 0 0 0 7.2z M18.4 19.6a3.6 3.6 0 1 0 0-7.2 3.6 3.6 0 0 0 0 7.2z M5.6 16l3.8-5.6h4.8 M9.4 10.4l4.4 5.6 M14.2 10.4h2.6 M18.4 16l-1.6-5.6 M16.6 6.4a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3z";
        case "swim":
            return "M2.6 18.8c1.8 0 1.8 1.4 3.6 1.4s1.8-1.4 3.6-1.4 1.8 1.4 3.6 1.4 1.8-1.4 3.6-1.4 1.8 1.4 3.6 1.4 M16.6 8.6a1.9 1.9 0 1 0 0-3.8 1.9 1.9 0 0 0 0 3.8z M4.2 13.8l5.2-3.2 4.4 2.8 3.4-2.2";
        case "walk":
            return "M13.4 5a1.8 1.8 0 1 0 0-3.6 1.8 1.8 0 0 0 0 3.6z M9.4 21.6l2.4-6.2-1.8-3.4 1-4 3.4-1 2 3.2 2.6 1.2 M11.8 15.4l3.2 6.2";
        case "hike":
            return "M13.8 5a1.8 1.8 0 1 0 0-3.6 1.8 1.8 0 0 0 0 3.6z M8.4 21.6l3-5.6-2-3.2 1.2-3.8 3.4-1.2 2.2 3 2.6 1.2 M11.4 16l2.8 5.6 M19.4 8.4v13.2";
        case "strength":
            return "M3.4 9.4h2.4v5.2H3.4z M6.8 7.6h2.8v8.8H6.8z M14.4 7.6h2.8v8.8h-2.8z M18.2 9.4h2.4v5.2h-2.4z M9.6 12h4.8";
        case "yoga":
            return "M12 5.4a1.9 1.9 0 1 0 0-3.8 1.9 1.9 0 0 0 0 3.8z M12 7.4v5.4 M12 12.8c-3 0-5.4 2.5-5.4 5.6h10.8c0-3.1-2.4-5.6-5.4-5.6z M6.6 18.4l-3.4 2 M17.4 18.4l3.4 2";
        case "cardio":
            return "M12 20.4C10.4 19.2 4.9 15.4 4.9 11.2A3.9 3.9 0 0 1 12 8.9a3.9 3.9 0 0 1 7.1 2.3c0 4.2-5.5 8-7.1 9.2z M6.4 12.2h2.9l1.4-2.7 2 5.1 1.4-2.4h3.3";
        case "tennis":
            return "M14 2.8a5.4 5.4 0 1 0 0 10.8 5.4 5.4 0 0 0 0-10.8z M10.2 11.8l-6 7.7 1.7 1.5 6.4-7.2";
        case "basketball":
            return "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M3 12h18 M12 3v18 M5.6 5.6c3.4 3.4 3.4 9.4 0 12.8 M18.4 5.6c-3.4 3.4-3.4 9.4 0 12.8";
        case "soccer":
            return "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M12 7.4l3.8 2.8-1.5 4.4H9.7L8.2 10.2z";
        case "row":
            return "M2.8 19.2c1.8 0 1.8 1.4 3.6 1.4s1.8-1.4 3.6-1.4 1.8 1.4 3.6 1.4 1.8-1.4 3.6-1.4 1.8 1.4 3.6 1.4 M4.6 16.6L15.8 5.4 M16.4 2.6l4.8 4.8-2.4 2.4-4.8-4.8z M8.4 12.8l3 3";
        case "ski":
            return "M15 4.6a1.8 1.8 0 1 0 0-3.6 1.8 1.8 0 0 0 0 3.6z M3.4 19l16.2-5.8 M9.6 12.4l3.2-2.8 3.4 1.8 2.2 3.2 M12.8 9.6l-1.8-3.4 M5.4 21.2l14.2-5";
        case "paddle":
            return "M12 2.6v18.8 M12 2.6c-1.8 1-2.9 2.7-2.9 4.3s1.1 2.7 2.9 2.7 2.9-1.1 2.9-2.7-1.1-3.3-2.9-4.3z M12 21.4c1.8-1 2.9-2.7 2.9-4.3s-1.1-2.7-2.9-2.7-2.9 1.1-2.9 2.7 1.1 3.3 2.9 4.3z";
        case "treadmill":
            return "M2.6 20.4h15.6 M4.4 20.4l1.8-4.8h11.6l-1.4 4.8 M17.6 15.6l2.6-9.6 M18.8 5.2h2.6 M9 12.6V8.4l2.8-1.3 M13.4 6.2a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3z";
        case "multisport":
            return "M12 3.4l8.4 4.4L12 12.2 3.6 7.8z M3.6 12l8.4 4.4 8.4-4.4 M3.6 16.2l8.4 4.4 8.4-4.4";

        // ---- ui ----------------------------------------------------------
        case "calendar":
            return "M4.4 6.6h15.2v13.8H4.4z M4.4 10.8h15.2 M8.6 3.6v4 M15.4 3.6v4";
        case "clock":
            return "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M12 6.8v5.6l3.4 2";
        case "info":
            return "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M12 11.2v5.4 M12 7.4v.4";
        case "settings":
            return "M12 15.2a3.2 3.2 0 1 0 0-6.4 3.2 3.2 0 0 0 0 6.4z M12 2.8v2.8 M12 18.4v2.8 M2.8 12h2.8 M18.4 12h2.8 M5.4 5.4l2 2 M16.6 16.6l2 2 M18.6 5.4l-2 2 M7.4 16.6l-2 2";
        case "phone_off":
            return "M6.2 3.4l2.9 4.9-2 2a12.4 12.4 0 0 0 5.2 5.2l2-2 4.9 2.9-2 3c-6.2 1-13.9-6.7-12.9-12.9z M3.2 3.2l17.6 17.6";
        case "music":
            return "M9.4 18.2V5.6l9.2-2v12.6 M9.4 9.6l9.2-2 M7 20.6a2.4 2.4 0 1 0 0-4.8 2.4 2.4 0 0 0 0 4.8z M16.2 18.6a2.4 2.4 0 1 0 0-4.8 2.4 2.4 0 0 0 0 4.8z";
        case "weather":
            return "M8.4 9.4a4 4 0 0 1 7.8 1.2h.6a3.4 3.4 0 0 1 0 6.8H7.8a4 4 0 0 1-.6-8z M6.2 6.2L4.8 4.8 M12 4.2V2.4 M18.2 6l1.4-1.4";
        case "map":
            return "M9 4.4L3.6 6.6v13L9 17.4l6 2.2 5.4-2.2v-13L15 6.6z M9 4.4v13 M15 6.6v13";
        case "lungs":
            return "M12 3.4v9.4 M12 12.8c0-2.2-1.4-3.6-2.8-4.6-1.6-1.2-3.8-.4-4.2 1.8-.5 2.6-.6 5.4-.2 8 .2 1.4 1.8 2 3 1.4l2.6-1.4c1-.6 1.6-1.6 1.6-2.8z M12 12.8c0-2.2 1.4-3.6 2.8-4.6 1.6-1.2 3.8-.4 4.2 1.8.5 2.6.6 5.4.2 8-.2 1.4-1.8 2-3 1.4l-2.6-1.4c-1-.6-1.6-1.6-1.6-2.8z";
        case "sleep_score":
            return "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M15.6 14.4A5.2 5.2 0 0 1 9.6 8.4a5.2 5.2 0 1 0 6 6z";
        case "trend_up":
            return "M3.4 17.4l5.6-5.6 3.6 3.6 7.6-7.6 M15.4 7.8h5.2v5.2";
        case "trend_down":
            return "M3.4 7.8l5.6 5.6 3.6-3.6 7.6 7.6 M15.4 17.4h5.2v-5.2";
        }
        return "";
    }

    // QtQuick.Shapes draws nothing inside Ubuntu.Components' MainView on this
    // platform — verified on device: the identical ShapePath renders standalone
    // and stays blank under MainView, which is why every icon was missing. The
    // svg image plugin handles the same path data, so the marks above are used
    // verbatim and only the rasteriser changes.
    Image {
        anchors.fill: parent
        visible: root.path.length > 0
        smooth: true
        // Rasterise at device pixels so the stroke stays crisp.
        sourceSize.width: Math.max(1, Math.round(root.size))
        sourceSize.height: Math.max(1, Math.round(root.size))
        source: root.path.length > 0 ? root.svgSource : ""
    }

    readonly property string svgSource:
        "data:image/svg+xml;utf8," +
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">' +
        '<path d="' + path + '" fill="none"' +
        ' stroke="' + rgbOf(color) + '" stroke-opacity="' + color.a.toFixed(3) + '"' +
        ' stroke-width="' + weight + '"' +
        ' stroke-linecap="round" stroke-linejoin="round"/></svg>'

    // Qt stringifies a color as #aarrggbb, which SVG does not understand, so
    // the channels are written out and the alpha travels as stroke-opacity.
    function rgbOf(c) {
        return "rgb(" + Math.round(c.r * 255) + "," +
                        Math.round(c.g * 255) + "," +
                        Math.round(c.b * 255) + ")";
    }
}
