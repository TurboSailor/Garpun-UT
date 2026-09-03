import QtQuick 2.12
import "../js/I18n.js" as I18n
import "../js/Fmt.js" as Fmt

// Coverage harness: every key present in the English table must also exist in
// the Russian one, and a spot sample of both is printed for eyeballing.
//   QT_QPA_PLATFORM=offscreen qmlscene dev/i18ncheck.qml
Item {
    Component.onCompleted: {
        var en = I18n.DICT_EN;
        var ru = I18n.DICT_RU;

        var enKeys = Object.keys(en);
        var ruKeys = Object.keys(ru);
        var missingRu = [];
        var missingEn = [];
        var untranslated = [];

        for (var i = 0; i < enKeys.length; i++) {
            var k = enKeys[i];
            if (!ru.hasOwnProperty(k)) missingRu.push(k);
            else if (ru[k] === en[k] && !/^(sum\.swolf|metric\.body_energy|lock\.body_battery$|accent\.|unit\.pct)/.test(k))
                untranslated.push(k);
        }
        for (var j = 0; j < ruKeys.length; j++) {
            if (!en.hasOwnProperty(ruKeys[j])) missingEn.push(ruKeys[j]);
        }

        console.log("EN keys: " + enKeys.length + "  RU keys: " + ruKeys.length);
        console.log("MISSING_RU (" + missingRu.length + "): " + missingRu.join(", "));
        console.log("MISSING_EN (" + missingEn.length + "): " + missingEn.join(", "));
        console.log("IDENTICAL (" + untranslated.length + "): " + untranslated.join(", "));

        // Placeholder arity: every %N in English must appear in Russian too.
        var badArgs = [];
        for (var m = 0; m < enKeys.length; m++) {
            var key = enKeys[m];
            if (!ru.hasOwnProperty(key)) continue;
            var ne = (en[key].match(/%\d/g) || []).sort().join("");
            var nr = (ru[key].match(/%\d/g) || []).sort().join("");
            if (ne !== nr) badArgs.push(key + " [en:" + ne + " ru:" + nr + "]");
        }
        console.log("ARG_MISMATCH (" + badArgs.length + "): " + badArgs.join(", "));

        Qt.quit();
    }
}
