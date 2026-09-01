import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Port of views/PulseRingView.java: 26dp track, gradient sweep starting at
// -90 degrees, a darker knob at the head, a second darker ring once the goal
// is beaten, and the 820ms overshoot entry animation.
Item {
    id: root

    property real progress: 0
    property color from: Pulse.accent
    property color to: Pulse.ringSteps
    property real thickness: units.dp(24)
    property bool animate: true

    // Driven value; overshoot is intentional and matches OvershootInterpolator(1.4).
    property real shown: 0

    implicitWidth: units.gu(19)
    implicitHeight: implicitWidth

    default property alias centerData: centre.data

    onProgressChanged: retarget()
    Component.onCompleted: retarget()

    function retarget() {
        if (!animate) {
            shown = progress;
            return;
        }
        entry.stop();
        entry.to = progress;
        entry.start();
    }

    NumberAnimation {
        id: entry
        target: root
        property: "shown"
        duration: Pulse.ring
        easing.type: Easing.OutBack
        easing.overshoot: 1.4
    }

    onShownChanged: canvas.requestPaint()
    Connections {
        target: Pulse
        onAccentChanged: canvas.requestPaint()
        onDarkChanged: canvas.requestPaint()
    }

    Canvas {
        id: canvas
        anchors.fill: parent
        antialiasing: true
        renderTarget: Canvas.Image

        onPaint: {
            var ctx = getContext("2d");
            ctx.reset();
            var w = width, h = height;
            var t = root.thickness;
            var cx = w / 2, cy = h / 2;
            var r = Math.min(w, h) / 2 - t / 2;
            if (r <= 0) return;

            var p = Math.max(0, root.shown);
            var over = p > 1;

            ctx.lineWidth = t;
            ctx.lineCap = "butt";

            // track
            ctx.beginPath();
            ctx.strokeStyle = Pulse.cardAlt;
            ctx.arc(cx, cy, r, 0, Math.PI * 2);
            ctx.stroke();

            // completed lap sits under the live sweep
            if (over) {
                ctx.beginPath();
                ctx.strokeStyle = Pulse.shade(root.from, 0.55);
                ctx.arc(cx, cy, r, 0, Math.PI * 2);
                ctx.stroke();
            }

            var frac = over ? Math.min(1, p - 1) : Math.min(1, p);
            if (frac < 0) frac = 0;

            if (frac > 0.0005) {
                var grad = ctx.createLinearGradient(0, 0, w, h);
                grad.addColorStop(0, root.from);
                grad.addColorStop(1, root.to);
                ctx.beginPath();
                ctx.lineCap = "round";
                ctx.strokeStyle = grad;
                var start = -Math.PI / 2;
                ctx.arc(cx, cy, r, start, start + frac * Math.PI * 2);
                ctx.stroke();

                // knob at the head of the sweep
                var a = start + frac * Math.PI * 2;
                ctx.beginPath();
                ctx.fillStyle = over ? Pulse.tint(root.from, 0.55) : Pulse.shade(root.from, 0.6);
                ctx.arc(cx + Math.cos(a) * r, cy + Math.sin(a) * r, t * 0.22, 0, Math.PI * 2);
                ctx.fill();
            }
        }
    }

    Item {
        id: centre
        anchors.centerIn: parent
        width: parent.width - 2 * root.thickness - units.gu(1)
        height: width
    }
}
