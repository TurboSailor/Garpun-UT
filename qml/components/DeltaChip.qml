import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../js/Fmt.js" as Fmt
import "../js/I18n.js" as I18n

// "vs 7-day average" indicator. Flat is a legitimate answer and gets its own
// wording instead of a misleading 0.
Row {
    id: root

    property real delta: 0
    property real reference: 0
    property string unit: ""
    property bool inverted: false
    property int decimals: 0

    // docs §2.11: a swing under max(1, avg/20) is noise, not a trend.
    readonly property real threshold: Math.max(1, Math.abs(reference) / 20)
    readonly property bool flat: Math.abs(delta) <= threshold
    readonly property bool good: inverted ? delta < 0 : delta > 0

    spacing: units.dp(3)

    Glyph {
        visible: !root.flat
        anchors.verticalCenter: parent.verticalCenter
        name: root.delta > 0 ? "chevron" : "chevron"
        rotation: root.delta > 0 ? -90 : 90
        size: units.gu(1.5)
        weight: 2.6
        color: root.good ? Pulse.mint : Pulse.ringCal
    }

    Label {
        anchors.verticalCenter: parent.verticalCenter
        text: root.flat ? I18n.t("action.steady") : Fmt.trimNum(Math.abs(root.delta), root.decimals) + (root.unit ? " " + root.unit : "")
        color: root.flat ? Pulse.textDim : (root.good ? Pulse.mint : Pulse.ringCal)
        font.family: Pulse.face
        font.pixelSize: Pulse.micro
        font.weight: Font.DemiBold
    }
}
