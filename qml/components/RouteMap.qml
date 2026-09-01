import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Track outline drawn straight from the GPS points. No tiles, no network —
// the shape of the route is the useful part offline.
Canvas {
    id: root

    property var track: []
    property color hue: Pulse.accent

    antialiasing: true
    renderTarget: Canvas.Image

    onTrackChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()

    readonly property int points: {
        var n = 0;
        var t = track || [];
        for (var i = 0; i < t.length; i++)
            if (t[i].lat && t[i].lon) n++;
        return n;
    }

    onPaint: {
        var ctx = getContext("2d");
        ctx.reset();
        var t = track || [];
        if (points < 2 || width <= 0 || height <= 0) return;

        var minLat = Infinity, maxLat = -Infinity, minLon = Infinity, maxLon = -Infinity;
        for (var i = 0; i < t.length; i++) {
            if (!t[i].lat || !t[i].lon) continue;
            if (t[i].lat < minLat) minLat = t[i].lat;
            if (t[i].lat > maxLat) maxLat = t[i].lat;
            if (t[i].lon < minLon) minLon = t[i].lon;
            if (t[i].lon > maxLon) maxLon = t[i].lon;
        }
        // Longitude degrees shrink with latitude; without this correction the
        // track looks stretched east-west.
        var midLat = (minLat + maxLat) / 2;
        var kx = Math.cos(midLat * Math.PI / 180);
        var spanX = Math.max(1e-9, (maxLon - minLon) * kx);
        var spanY = Math.max(1e-9, maxLat - minLat);

        var pad = units.gu(1.5);
        var w = width - 2 * pad, h = height - 2 * pad;
        var scale = Math.min(w / spanX, h / spanY);
        var offX = pad + (w - spanX * scale) / 2;
        var offY = pad + (h - spanY * scale) / 2;

        function px(p) { return offX + (p.lon - minLon) * kx * scale; }
        function py(p) { return offY + (maxLat - p.lat) * scale; }

        ctx.beginPath();
        var started = false;
        var first = null, last = null;
        for (var j = 0; j < t.length; j++) {
            var p = t[j];
            if (!p.lat || !p.lon) continue;
            if (!started) { ctx.moveTo(px(p), py(p)); started = true; first = p; }
            else ctx.lineTo(px(p), py(p));
            last = p;
        }
        ctx.lineWidth = units.dp(3);
        ctx.lineJoin = "round";
        ctx.lineCap = "round";
        ctx.strokeStyle = root.hue;
        ctx.stroke();

        if (first) {
            ctx.beginPath();
            ctx.fillStyle = Pulse.mint;
            ctx.arc(px(first), py(first), units.dp(4), 0, Math.PI * 2);
            ctx.fill();
        }
        if (last) {
            ctx.beginPath();
            ctx.fillStyle = Pulse.ringHr;
            ctx.arc(px(last), py(last), units.dp(4), 0, Math.PI * 2);
            ctx.fill();
        }
    }
}
