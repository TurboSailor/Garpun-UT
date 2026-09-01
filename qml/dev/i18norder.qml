import QtQuick 2.12
import "../js/I18n.js" as I18n

// The tab strip and the accent list are built declaratively from I18n.t(), and
// those bindings run before any Component.onCompleted. This checks the language
// is already resolved by then — that is what makes building them declaratively
// safe, and it is exactly what used to leave English tabs under a Russian UI.
//
//   QT_QPA_PLATFORM=offscreen qmlscene dev/i18norder.qml
//   LANGUAGE=ru_RU:ru LANG=ru_RU.UTF-8 QT_QPA_PLATFORM=offscreen qmlscene dev/i18norder.qml
Item {
    // Evaluated during construction, i.e. before onCompleted runs.
    readonly property string atBindingTime: I18n.t("tab.today")

    Component.onCompleted: {
        var atCompletion = I18n.t("tab.today");
        console.log("LANG          =", I18n.current());
        console.log("AT BINDING    =", atBindingTime);
        console.log("AT COMPLETION =", atCompletion);
        var ok = atBindingTime === atCompletion && atBindingTime !== "tab.today";
        console.log(ok ? "ORDER OK"
                       : "ORDER BROKEN: binding saw an untranslated or stale value");
        Qt.quit();
    }
}
