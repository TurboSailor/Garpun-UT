# Garpun-UT

Порт [Pulse](https://zachy.cc) — Garmin-only форка [Gadgetbridge](https://codeberg.org/Freeyourgadget/Gadgetbridge) — на **Ubuntu Touch 24.04** (Lomiri, aarch64).

Оригинал Android (`cc.zachy.pulse`) здесь не хранится; это нативная переписка:

| Слой | Технология |
|---|---|
| Демон `pulsed` | Go · BLE через BlueZ D-Bus · протокол Garmin GFDI · парсер/энкодер FIT · SQLite |
| Фронтенд | QML / Ubuntu.Components 1.3 |
| Упаковка | click-пакет `cc.zachy.pulse` |

## Структура

```
backend/    Go-модуль: pulsed + pulsectl
qml/        QML-фронтенд (Today / Health / Sleep / Fitness / Device)
click/      манифест, AppArmor, .desktop, run.sh
scripts/    build.sh / deploy.sh / logs.sh
docs/       извлечённые из оригинала спецификации протокола
testdata/   реальные FIT-файлы с Forerunner 255
```

Локальный справочник исходников Android при необходимости кладётся рядом как
`pulse-main/` (в `.gitignore`, в репозиторий не входит).

## Сборка и деплой

Телефон должен быть в `adb devices`. На macOS `click` нет — упаковка
происходит на устройстве.

```bash
make click     # cross-compile arm64 + сборка .click на телефоне
make deploy    # click install --force --allow-unauthenticated
make logs      # journal + pulsed.log
```

Пароль sudo на телефоне по умолчанию читается из `PULSE_SUDO_PASS`
(см. `scripts/deploy.sh`).

## Жизненный цикл демона

`run.sh` поднимает `pulsed` (и `pulse-wdnotify`) не своим потомком, а отдельными
транзиентными юнитами `systemd --user`:

```bash
systemctl --user status pulse-pulsed pulse-wdnotify
```

Это обязательно, а не стилистика: Lomiri усыпляет фоновое приложение через
SIGSTOP всему cgroup его app-launch-юнита, а затем гасит юнит целиком
(`KillMode=control-group`). Демон-потомок (пусть и через `setsid`) остаётся в
том же cgroup — замерзает посреди синхронизации вместе с приложением. Свой юнит
= свой cgroup, поэтому синхронизация и уведомления живут при закрытом UI.

## Состояние

Проверено на Nothing Phone 1 (UT 24.04) + Garmin Forerunner 255:

- BLE-сканирование, сопряжение и подключение через BlueZ D-Bus
- GFDI-транспорт v2 (multi-link), COBS, CRC, кадрирование
- Инициализация сессии: device info → auth → capabilities → sync ready
- Синхронизация файлов: листинг, скачивание, ARCHIVE, пропуск уже скачанных
- FIT → SQLite → аналитика дашборда (шаги, пульс, стресс, body battery, сон)
- Погода Open-Meteo → FIT weather payload на часы
- Уведомления freedesktop / Waydroid / ofono / MPRIS → часы
- QML UI: 5 вкладок + детали, компиляция на устройстве `ALL OK`

## Лицензия

AGPLv3, как у Gadgetbridge / Pulse.
