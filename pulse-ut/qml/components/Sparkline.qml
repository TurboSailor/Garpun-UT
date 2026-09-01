import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// 7-point trend line with a soft fill and a dot on the newest sample.
Canvas {
    id: root

    property var values: []
    property color hue: Pulse.accent
    property bool fill: true
    property bool dot: true
    property real weight: units.dp(2)

    antialiasing: true
    renderTarget: Canvas.Image

    onValuesChanged: requestPaint()
    onHueChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()

    onPaint: {
        var ctx = getContext("2d");
        ctx.reset();
        var v = values || [];
        var n = v.length;
        if (n === 0 || width <= 0 || height <= 0)
            return;

        var min = Infinity, max = -Infinity;
        for (var i = 0; i < n; i++) {
            if (v[i] < min) min = v[i];
            if (v[i] > max) max = v[i];
        }
        if (!isFinite(min)) return;
        var span = max - min;
        if (span <= 0) { min -= 1; max += 1; span = 2; }

        var pad = weight + units.dp(2);
        var w = width, h = height - 2 * pad;
        var stepX = n > 1 ? w / (n - 1) : 0;

        function px(i) { return n > 1 ? i * stepX : w / 2; }
        function py(i) { return pad + h - (v[i] - min) / span * h; }

        if (fill && n > 1) {
            ctx.beginPath();
            ctx.moveTo(px(0), height);
            for (var a = 0; a < n; a++) ctx.lineTo(px(a), py(a));
            ctx.lineTo(px(n - 1), height);
            ctx.closePath();
            var g = ctx.createLinearGradient(0, 0, 0, height);
            g.addColorStop(0, Pulse.alpha(hue, 0.28));
            g.addColorStop(1, Pulse.alpha(hue, 0.0));
            ctx.fillStyle = g;
            ctx.fill();
        }

        ctx.beginPath();
        ctx.lineWidth = weight;
        ctx.lineJoin = "round";
        ctx.lineCap = "round";
        ctx.strokeStyle = hue;
        for (var b = 0; b < n; b++) {
            if (b === 0) ctx.moveTo(px(b), py(b));
            else ctx.lineTo(px(b), py(b));
        }
        ctx.stroke();

        if (dot) {
            ctx.beginPath();
            ctx.fillStyle = hue;
            ctx.arc(px(n - 1), py(n - 1), weight * 1.5, 0, Math.PI * 2);
            ctx.fill();
        }
    }
}
