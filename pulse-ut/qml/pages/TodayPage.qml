import QtQuick 2.12
import Ubuntu.Components 1.3
import "../theme"
import "../store"
import "../components"
import "../js/Fmt.js" as Fmt

Item {
    id: page

    signal openNotifications()
    signal openWorkout(var workout)
    signal openTab(string key)

    readonly property var t: Store.today
    readonly property bool waiting: Store.today === null && (Store.todayLoading || !Store.everAnswered)

    // GET /api/today carries goals under short keys; /api/settings uses the
    // long ones. Resolve daemon value first, settings second, spec default last.
    function goal(key) {
        var g = t && t.goals ? t.goals : null;
        var s = Store.settings;
        function pick(a, b, c) {
            if (a > 0) return a;
            if (b > 0) return b;
            return c;
        }
        switch (key) {
        case "steps":            return pick(g ? g.steps : 0, t ? t.stepsGoal : 0, s.stepsGoal > 0 ? s.stepsGoal : 10000);
        case "sleepMinutes":     return pick(g ? g.sleepMinutes : 0, s.sleepGoalMinutes, 480);
        case "activeCalories":   return pick(g ? g.activeCalories : 0, s.activeCaloriesGoal, 350);
        case "distanceM":        return pick(g ? g.distanceM : 0, s.distanceGoalM, 5000);
        case "activeMinutes":    return pick(g ? g.activeMinutes : 0, s.activeMinutesGoal, 60);
        case "intensityMinutes": return pick(g ? g.intensityMinutes : 0, s.intensityGoal, 30);
        }
        return 1;
    }
    function num(path, fallback) {
        return t && t[path] !== undefined && t[path] !== null ? t[path] : fallback;
    }
    function sub(group, key) {
        if (!t || !t[group]) return 0;
        var v = t[group][key];
        return (v === undefined || v === null) ? 0 : v;
    }

    readonly property real steps: num("steps", 0)
    readonly property real stepsGoal: goal("steps")
    readonly property real stepFactor: stepsGoal > 0 ? steps / stepsGoal : 0

    Screen {
        anchors.fill: parent

        PageHead {
            width: parent.width
            kicker: Fmt.prettyDate(Store.date)
            title: Fmt.greeting(new Date().getHours())
            trailingGlyph: "bell"
            trailingBadge: Store.notifications ? Store.notifications.length : 0
            onTrailing: page.openNotifications()
        }

        DateNav { width: parent.width }

        StatusStrip { width: parent.width }

        // ---- hero -------------------------------------------------------
        Card {
            width: parent.width
            padding: Pulse.l

            Item {
                width: parent.width
                height: Math.max(ring.height, sideCol.height)

                RingGauge {
                    id: ring
                    anchors.left: parent.left
                    anchors.verticalCenter: parent.verticalCenter
                    width: Math.min(units.gu(19), parent.width * 0.52)
                    height: width
                    thickness: units.dp(22)
                    progress: page.waiting ? 0 : page.stepFactor
                    from: Pulse.accent
                    to: Pulse.ringSteps

                    Column {
                        anchors.centerIn: parent
                        spacing: units.dp(2)

                        Label {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: page.waiting ? "\u2013" : Fmt.thousands(page.steps)
                            color: Pulse.text
                            font.family: Pulse.face
                            font.pixelSize: page.steps >= 100000 ? Pulse.title : Pulse.headline
                            font.weight: Font.DemiBold
                        }
                        Label {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: "of " + Fmt.thousands(page.stepsGoal)
                            color: Pulse.textDim
                            font.family: Pulse.face
                            font.pixelSize: Pulse.caption
                        }
                    }
                }

                Column {
                    id: sideCol
                    anchors.left: ring.right
                    anchors.leftMargin: Pulse.l
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Pulse.m

                    Label {
                        text: "STEPS"
                        color: Pulse.textDim
                        font.family: Pulse.face
                        font.pixelSize: Pulse.micro
                        font.weight: Font.DemiBold
                        font.letterSpacing: units.dp(1.4)
                    }

                    Label {
                        width: parent.width
                        wrapMode: Text.WordWrap
                        color: Pulse.text
                        font.family: Pulse.face
                        font.pixelSize: Pulse.body
                        lineHeight: 1.2
                        text: {
                            if (page.waiting) return "Reading today\u2026";
                            if (page.steps <= 0) return "No steps recorded yet today.";
                            if (page.stepFactor >= 1) return "Goal beaten by " +
                                Fmt.thousands(page.steps - page.stepsGoal) + " steps.";
                            return Fmt.thousands(page.stepsGoal - page.steps) + " steps to go.";
                        }
                    }

                    // streak plaque
                    Rectangle {
                        width: parent.width
                        height: units.gu(5.5)
                        radius: Pulse.radiusTile
                        color: Pulse.cardAlt

                        Glyph {
                            id: streakIcon
                            anchors.left: parent.left
                            anchors.leftMargin: Pulse.m
                            anchors.verticalCenter: parent.verticalCenter
                            name: "flame"
                            size: units.gu(2.25)
                            color: page.sub("streak", "current") > 0 ? Pulse.ringCal : Pulse.textDim
                        }

                        Column {
                            anchors.left: streakIcon.right
                            anchors.leftMargin: Pulse.s
                            anchors.verticalCenter: parent.verticalCenter
                            spacing: 0

                            Label {
                                text: page.sub("streak", "current") + (page.sub("streak", "current") === 1 ? " day" : " days")
                                color: Pulse.text
                                font.family: Pulse.face
                                font.pixelSize: Pulse.subtitle
                                font.weight: Font.DemiBold
                            }
                            Label {
                                text: page.sub("streak", "best") > 0
                                      ? "best " + page.sub("streak", "best")
                                      : "start a streak"
                                color: Pulse.textDim
                                font.family: Pulse.face
                                font.pixelSize: Pulse.micro
                            }
                        }
                    }
                }
            }
        }

        // ---- tiles -------------------------------------------------------
        Grid {
            id: tiles
            width: parent.width
            columns: 2
            spacing: Pulse.m

            readonly property real cell: (width - spacing) / 2

            MetricTile {
                width: tiles.cell
                label: "Calories"
                glyph: "flame"
                hue: Pulse.ringHr
                loading: page.waiting
                value: Fmt.thousands(page.num("activeCalories", 0))
                unit: "kcal"
                caption: page.num("restingCalories", 0) > 0
                         ? "+" + Fmt.thousands(page.num("restingCalories", 0)) + " resting" : ""
                factor: page.num("activeCalories", 0) / page.goal("activeCalories")
            }

            MetricTile {
                width: tiles.cell
                label: "Distance"
                glyph: "route"
                hue: Pulse.accent
                loading: page.waiting
                value: Fmt.distanceShort(page.num("distanceM", 0), Store.settings.units)
                unit: Fmt.distanceUnit(Store.settings.units)
                caption: "goal " + Fmt.distance(page.goal("distanceM"), Store.settings.units)
                factor: page.num("distanceM", 0) / page.goal("distanceM")
            }

            MetricTile {
                width: tiles.cell
                label: "Active time"
                glyph: "bolt"
                hue: Pulse.mint
                loading: page.waiting
                value: page.num("activeMinutes", 0) > 0 ? Fmt.durationTrim(page.num("activeMinutes", 0)) : "\u2013"
                caption: "intensity " + page.sub("intensityMinutes", "today") +
                         " \u00b7 week " + page.sub("intensityMinutes", "week")
                factor: page.num("activeMinutes", 0) / page.goal("activeMinutes")
            }

            MetricTile {
                width: tiles.cell
                label: "Heart rate"
                glyph: "pulse"
                hue: Pulse.ringHr
                loading: page.waiting
                value: page.sub("heartRate", "latest") > 0 ? "" + page.sub("heartRate", "latest") : "\u2013"
                unit: "bpm"
                caption: page.sub("heartRate", "resting") > 0
                         ? "resting " + page.sub("heartRate", "resting")
                         : (page.sub("heartRate", "max") > 0
                            ? page.sub("heartRate", "min") + "\u2013" + page.sub("heartRate", "max") : "")
                factor: Pulse.metricFactor("heart_rate", page.sub("heartRate", "latest"))
                onClicked: page.openTab("health")
            }

            MetricTile {
                width: tiles.cell
                label: "Body Battery"
                glyph: "battery"
                hue: Pulse.mint
                loading: page.waiting
                value: page.sub("bodyEnergy", "latest") > 0 ? "" + page.sub("bodyEnergy", "latest") : "\u2013"
                caption: page.sub("bodyEnergy", "max") > 0
                         ? "range " + page.sub("bodyEnergy", "min") + "\u2013" + page.sub("bodyEnergy", "max") : ""
                factor: page.sub("bodyEnergy", "latest") / 100
                onClicked: page.openTab("health")
            }

            MetricTile {
                width: tiles.cell
                label: "Sleep"
                glyph: "moon"
                hue: Pulse.purple
                loading: page.waiting
                value: page.num("sleepMinutes", 0) > 0 ? Fmt.durationTrim(page.num("sleepMinutes", 0)) : "\u2013"
                caption: page.num("sleepScore", 0) > 0
                         ? "score " + page.num("sleepScore", 0) + " \u00b7 " + Pulse.sleepQuality(page.num("sleepScore", 0))
                         : "no night recorded"
                factor: page.num("sleepMinutes", 0) / page.goal("sleepMinutes")
                onClicked: page.openTab("sleep")
            }
        }

        // ---- recent workouts ---------------------------------------------
        SectionTitle {
            width: parent.width
            text: "Recent workouts"
            action: Store.workouts && Store.workouts.length > 0 ? "See all" : ""
            onActionTriggered: page.openTab("fitness")
        }

        Card {
            width: parent.width
            padding: Store.workouts && Store.workouts.length > 0 ? Pulse.s : Pulse.l
            spacing: 0

            EmptyState {
                width: parent.width
                visible: !Store.workouts || Store.workouts.length === 0
                glyph: "timer"
                title: Store.online ? "No workouts yet" : "Waiting for the daemon"
                hint: Store.online
                      ? "Record an activity on the watch, then sync to see it here."
                      : "Pulse cannot reach pulsed on 127.0.0.1:21830."
            }

            Repeater {
                model: Store.workouts ? Store.workouts.slice(0, 3) : []
                delegate: WorkoutRow {
                    width: parent.width
                    workout: modelData
                    divider: index < Math.min(3, Store.workouts.length) - 1
                    onClicked: page.openWorkout(modelData)
                }
            }
        }
    }
}
