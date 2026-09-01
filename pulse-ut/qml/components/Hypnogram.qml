import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Stage timeline for one night. Rows are ordered awake -> rem -> light -> deep
// so the graph reads as "how deep", top to bottom.
Canvas {
    id: root

    property var stages: []      // [{startMs, endMs, stage}]
    property real startMs: 0
    property real endMs: 0

    readonly property var rows: ["awake", "rem", "light", "deep"]

    antialiasing: true
    renderTarget: Canvas.Image
    height: units.gu(11)

    // Stage colours come from the theme, so a light/dark switch must repaint.
    readonly property color tick: Pulse.textDim
    onTickChanged: requestPaint()
    onStagesChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()

    onPaint: {
        var ctx = getContext("2d");
        ctx.reset();
        var list = stages || [];
        if (list.length === 0 || width <= 0)
            return;

        var t0 = startMs > 0 ? startMs : list[0].startMs;
        var t1 = endMs > 0 ? endMs : list[list.length - 1].endMs;
        var span = t1 - t0;
        if (span <= 0) return;

        var gap = units.dp(3);
        var rowH = (height - gap * (rows.length - 1)) / rows.length;

        // faint guide per row keeps the empty parts of the night legible
        for (var r = 0; r < rows.length; r++) {
            var gy = r * (rowH + gap) + rowH / 2 - units.dp(0.5);
            ctx.fillStyle = Pulse.alpha(Pulse.textDim, 0.12);
            ctx.fillRect(0, gy, width, units.dp(1));
        }

        for (var i = 0; i < list.length; i++) {
            var seg = list[i];
            var idx = rows.indexOf(seg.stage);
            if (idx < 0) continue;
            var x = (seg.startMs - t0) / span * width;
            var w = Math.max(units.dp(2), (seg.endMs - seg.startMs) / span * width);
            if (x + w > width) w = width - x;
            if (w <= 0) continue;
            var y = idx * (rowH + gap);
            var rad = Math.min(rowH / 2, w / 2, units.dp(4));
            ctx.beginPath();
            ctx.fillStyle = Pulse.stageColor(seg.stage);
            ctx.moveTo(x + rad, y);
            ctx.lineTo(x + w - rad, y);
            ctx.quadraticCurveTo(x + w, y, x + w, y + rad);
            ctx.lineTo(x + w, y + rowH - rad);
            ctx.quadraticCurveTo(x + w, y + rowH, x + w - rad, y + rowH);
            ctx.lineTo(x + rad, y + rowH);
            ctx.quadraticCurveTo(x, y + rowH, x, y + rowH - rad);
            ctx.lineTo(x, y + rad);
            ctx.quadraticCurveTo(x, y, x + rad, y);
            ctx.closePath();
            ctx.fill();
        }
    }
}
