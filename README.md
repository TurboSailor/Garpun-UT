# Garpun-UT

Порт [Pulse](https://zachy.cc) — Garmin-only форка [Gadgetbridge](https://codeberg.org/Freeyourgadget/Gadgetbridge) — на **Ubuntu Touch 24.04** (Lomiri, aarch64).

Оригинал Android (`cc.zachy.pulse`) здесь не хранится; это нативная переписка
под пакетом `pulse.turbosailor`:

https://i.ibb.co/5xtcmN85/screenshot20260903-211143538.png

| Слой | Технология |
|---|---|
| Демон `pulsed` | Go · BLE через BlueZ D-Bus · протокол Garmin GFDI · парсер/энкодер FIT · SQLite |
| Фронтенд | QML / Ubuntu.Components 1.3 |
| Упаковка | click-пакет `pulse.turbosailor` |

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

`run.sh` поднимает `pulsed` не своим потомком, а отдельным транзиентным юнитом
`systemd --user`:

```bash
systemctl --user status pulse-pulsed
```

Это обязательно, а не стилистика: Lomiri усыпляет фоновое приложение через
SIGSTOP всему cgroup его app-launch-юнита, а затем гасит юнит целиком
(`KillMode=control-group`). Демон-потомок (пусть и через `setsid`) остаётся в
том же cgroup — замерзает посреди синхронизации вместе с приложением. Свой юнит
= свой cgroup, поэтому синхронизация и уведомления живут при закрытом UI.

## Уведомления Android: чужая работа

Pulse **не** опрашивает контейнер Waydroid и ничего не постит в шторку. Этим
занимается отдельное приложение
[WaydNotif](https://github.com/TurboSailor/WaydNotif-UT) (`waydnotif.turbosailor`):
оно кладёт Android-уведомления в штатный список Ubuntu Touch через
`com.lomiri.Postal`, а `pulsed` наблюдает шторку по
`org.freedesktop.Notifications` и пересылает на часы — как и любое родное
уведомление. Один владелец на задачу: иначе каждое уведомление попадало бы в
шторку дважды и уходило на часы дважды.

Карточку релея демон распознаёт по имени пакета и разворачивает: имя
Android-приложения в карточке лежит в `summary`, поэтому на часах оно и
становится именем приложения, а не `waydnotif.turbosailor`. Тумблер
«Уведомления Android» в настройках фильтрует именно их.

По этой же причине у пакета больше нет хука `push`: постить через Postal
некому.

## Состояние

Проверено на Nothing Phone 1 (UT 24.04) + Garmin Forerunner 255:

- BLE-сканирование, сопряжение и подключение через BlueZ D-Bus
- Разрыв линка ловится сигналом BlueZ; реконнект с бэкоффом 2→64 с
- GFDI-транспорт v2 (multi-link), COBS, CRC, кадрирование
- Инициализация сессии: device info → auth → capabilities → sync ready
- Синхронизация файлов: листинг, скачивание, ARCHIVE, пропуск уже скачанных
  (дедуп по паре тип+номер, иначе файл чужого типа архивировался непрочитанным)
- FIT → SQLite → аналитика дашборда (шаги, пульс, стресс, body battery, сон)
- Погода Open-Meteo → FIT weather payload на часы
- Уведомления freedesktop / ofono / MPRIS → часы (Android — через WaydNotif);
  снятая на телефоне карточка удаляется и с часов, dismiss с часов закрывает
  её на телефоне
- QML UI: 5 вкладок + детали, компиляция на устройстве `ALL OK`

## Лицензия

AGPLv3, как у Gadgetbridge / Pulse.
