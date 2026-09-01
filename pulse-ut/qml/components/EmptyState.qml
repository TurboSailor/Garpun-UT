import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"

// Empty states always say what to do next, never "nothing here".
Column {
    id: root

    property string glyph: "watch"
    property string title: ""
    property string hint: ""
    property string action: ""
    signal actionTriggered()

    spacing: Pulse.m

    Glyph {
        name: root.glyph
        size: units.gu(4)
        color: Pulse.textDim
        weight: 1.5
    }

    Label {
        width: root.width
        text: root.title
        color: Pulse.text
        font.family: Pulse.face
        font.pixelSize: Pulse.subtitle
        font.weight: Font.DemiBold
        wrapMode: Text.WordWrap
    }

    Label {
        width: root.width
        visible: root.hint.length > 0
        text: root.hint
        color: Pulse.textDim
        font.family: Pulse.face
        font.pixelSize: Pulse.body
        lineHeight: 1.25
        wrapMode: Text.WordWrap
    }

    PillButton {
        visible: root.action.length > 0
        text: root.action
        kind: "primary"
        onClicked: root.actionTriggered()
    }
}
