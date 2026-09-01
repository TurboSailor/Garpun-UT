import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/Fmt.js" as Fmt

// Single-series chart used for workout traces and health detail. Values are
// plotted against their own index; gaps (null/0-for-optional) are skipped so a
// partial GPS trace does not drop to the floor.
Item {
    id: root

    property var values: []
    property color hue: Pulse.accent
    property string unit: ""
    property int decimals: 0
    property bool zeroIsGap: false

    readonly property real vmin: computed.min
    readonly property real vmax: computed.max

    property QtObject computed: QtObject {
        property real min: 0
        property real max: 0
        property int count: 0
    }

    height: units.gu(14)

    onValuesChanged: recompute()
    Component.onCompleted: recompute()

    function recompute() {
        var v = values || [];
        var mn = Infinity, mx = -Infinity, n = 0;
        for (var i = 0; i < v.length; i++) {
            var x = v[i];
            if (x === null || x === undefined || isNaN(x)) continue;
            if (zeroIsGap && x === 0) continue;
            if (x < mn) mn = x;
            if (x > mx) mx = x;
            n++;
        }
        computed.min = isFinite(mn) ? mn : 0;
        computed.max = isFinite(mx) ? mx : 0;
        computed.count = n;
        canvas.requestPaint();
    }

    Canvas {
        id: canvas
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.rightMargin: axis.width + Pulse.s
        antialiasing: true
        renderTarget: Canvas.Image

        onWidthChanged: requestPaint()
        onHeightChanged: requestPaint()

        onPaint: {
            var ctx = getContext("2d");
            ctx.reset();
            var v = root.values || [];
            if (root.computed.count < 2 || width <= 0 || height <= 0) return;

            var mn = root.computed.min, mx = root.computed.max;
            var span = mx - mn;
            if (span <= 0) { mn -= 1; span = 2; }

            var pad = units.dp(4);
            var h = height - 2 * pad;
            var stepX = width / (v.length - 1);

            function py(x) { return pad + h - (x - mn) / span * h; }

            // baseline
            ctx.fillStyle = Pulse.alpha(Pulse.textDim, 0.14);
            ctx.fillRect(0, height - units.dp(1), width, units.dp(1));

            var started = false;
            ctx.beginPath();
            for (var i = 0; i < v.length; i++) {
                var x = v[i];
                if (x === null || x === undefined || isNaN(x) || (root.zeroIsGap && x === 0)) {
                    started = false;
                    continue;
                }
                if (!started) { ctx.moveTo(i * stepX, py(x)); started = true; }
                else ctx.lineTo(i * stepX, py(x));
            }
            ctx.lineWidth = units.dp(2);
            ctx.lineJoin = "round";
            ctx.lineCap = "round";
            ctx.strokeStyle = root.hue;
            ctx.stroke();

            // fill under the trace
            ctx.lineTo(width, height);
            ctx.lineTo(0, height);
            ctx.closePath();
            var g = ctx.createLinearGradient(0, 0, 0, height);
            g.addColorStop(0, Pulse.alpha(root.hue, 0.22));
            g.addColorStop(1, Pulse.alpha(root.hue, 0.0));
            ctx.fillStyle = g;
            ctx.fill();
        }
    }

    Column {
        id: axis
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        width: units.gu(5)

        Label {
            text: root.computed.count > 0 ? Fmt.trimNum(root.vmax, root.decimals) : ""
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.micro
            horizontalAlignment: Text.AlignRight
            width: parent.width
        }
        Item { width: 1; height: axis.height - 2 * Pulse.micro - units.dp(6) }
        Label {
            text: root.computed.count > 0 ? Fmt.trimNum(root.vmin, root.decimals) : ""
            color: Pulse.textDim
            font.family: Pulse.face
            font.pixelSize: Pulse.micro
            horizontalAlignment: Text.AlignRight
            width: parent.width
        }
    }
}
