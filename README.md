# Garpun-UT

Порт [Pulse](https://github.com/) — форка Gadgetbridge для часов Garmin — на **Ubuntu Touch 24.04** (Lomiri, aarch64).

Оригинальное приложение — Android (Java/Kotlin, ~3800 файлов). Здесь оно переписывается
на нативный для Ubuntu Touch стек:

| Слой | Технология |
|---|---|
| Демон | Go, BLE через BlueZ D-Bus, протокол Garmin GFDI, парсер FIT, SQLite |
| Фронтенд | QML / Ubuntu.Components 1.3 |
| Упаковка | click-пакет `cc.zachy.pulse` |

## Структура

```
pulse-main/   исходный Android-проект Pulse (справочник, AGPLv3)
pulse-ut/     порт на Ubuntu Touch
  backend/    Go-демон pulsed + диагностический pulsectl
  qml/        QML-фронтенд
  click/      манифест, AppArmor-профиль, .desktop
  docs/       извлечённые из оригинала спецификации протокола
  testdata/   реальные FIT-файлы, снятые с Forerunner 255
```

## Состояние

Проверено на реальном железе (Nothing Phone 1 с Ubuntu Touch 24.04 + Garmin Forerunner 255):

- BLE-сканирование, сопряжение и подключение через BlueZ D-Bus
- GFDI-транспорт v2 (multi-link), COBS, CRC, кадрирование
- Полный цикл инициализации сессии (device info → auth → capabilities → sync ready)
- Синхронизация файлов с часов: листинг директории, скачивание, ARCHIVE
- Разбор FIT-файлов (шаги, пульс, стресс, body battery, дыхание, сон)

## Лицензия

Порт наследует **AGPLv3** оригинального Gadgetbridge/Pulse.
