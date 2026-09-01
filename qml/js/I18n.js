.pragma library

// Internationalization engine for Pulse QML UI.
// Supports English ("en") and Russian ("ru") with automatic detection
// from the system locale (Qt.locale().name / LANG / LANGUAGE).

var currentLang = "en";

var DICT_EN = {
    // Tabs
    "tab.today": "Today",
    "tab.health": "Health",
    "tab.sleep": "Sleep",
    "tab.fitness": "Fitness",
    "tab.device": "Device",

    // Common actions & buttons
    "action.connect": "Connect",
    "action.disconnect": "Disconnect",
    "action.sync": "Sync",
    "action.syncing": "Syncing",
    "action.sync_now": "Sync now",
    "action.find_watch": "Find my watch",
    "action.stop_ringing": "Stop ringing",
    "action.scan": "Scan",
    "action.stop": "Stop",
    "action.pair": "Pair",
    "action.paired": "Paired",
    "action.cancel": "Cancel",
    "action.confirm": "Confirm",
    "action.retry": "Retry",
    "action.see_all": "See all",
    "action.dismiss": "Dismiss",
    "action.steady": "steady",

    // Status strip
    "status.daemon_offline": "Pulse daemon offline",
    "status.start_daemon": "Start pulsed to see your data",
    "status.no_watch": "No watch paired",
    "status.open_device_scan": "Open Device to scan and pair",
    "status.connected": "Connected",
    "status.connecting": "Connecting…",
    "status.disconnected": "Disconnected",
    "status.synced_relative": "synced %1",
    "status.syncing_file": "Syncing file %1 · %2%",
    "status.syncing_ellipsis": "Syncing…",

    // Today Page
    "today.steps": "STEPS",
    "today.reading": "Reading today…",
    "today.no_steps": "No steps recorded yet today.",
    "today.goal_beaten": "Goal beaten by %1 steps.",
    "today.steps_to_go": "%1 steps to go.",
    "today.recent_workouts": "Recent workouts",
    "today.no_workouts_title": "No workouts yet",
    "today.no_workouts_hint": "Record an activity on the watch, then sync to see it here.",
    "today.daemon_wait_title": "Waiting for the daemon",
    "today.daemon_unreachable": "Pulse cannot reach pulsed on 127.0.0.1:21830.",

    // Metrics & Tiles
    "metric.steps": "Steps",
    "metric.distance": "Distance",
    "metric.activetime": "Active time",
    "metric.active_minutes": "Active minutes",
    "metric.calories": "Calories",
    "metric.intensity": "Intensity",
    "metric.sleep": "Sleep",
    "metric.heart_rate": "Heart rate",
    "metric.resting_hr": "Resting HR",
    "metric.body_energy": "Body Battery",
    "metric.stress": "Stress",
    "metric.spo2": "Blood oxygen",
    "metric.hrv": "HRV",
    "metric.respiration": "Respiration",

    // Sleep Page
    "sleep.title": "Sleep",
    "sleep.no_night_title": "No night recorded",
    "sleep.no_night_hint": "Wear the watch overnight, then sync. Nights are filed under the day you wake up.",
    "sleep.start_pulsed_hint": "Start pulsed on 127.0.0.1:21830 to read sleep data.",
    "sleep.score_label": "Sleep score",
    "sleep.goal_share": "of goal",
    "sleep.hypnogram": "Hypnogram",
    "sleep.no_stages": "No stage detail for this night",
    "sleep.last_7_nights": "Last 7 nights",
    "sleep.naps": "Naps",
    "sleep.avg_nights": "Average %1 over %2",
    "sleep.no_nights_yet": "No nights recorded yet",
    "sleep.deep": "Deep",
    "sleep.light": "Light",
    "sleep.rem": "REM",
    "sleep.awake": "Awake",
    "sleep.quality_excellent": "Excellent",
    "sleep.quality_good": "Good",
    "sleep.quality_fair": "Fair",
    "sleep.quality_poor": "Poor",

    // Health Page
    "health.kicker": "Trends",
    "health.title": "Health",
    "health.no_history_title": "No health history yet",
    "health.no_history_hint": "Wear the watch through a day and sync — heart rate, Body Battery, stress, SpO₂, HRV and respiration land here.",
    "health.next_sync_hint": "Start pulsed and this page fills in on the next sync.",
    "health.deltas_hint": "Deltas compare the latest value with the average of the preceding days.",
    "health.days_7": "7 days",
    "health.days_14": "14 days",
    "health.days_30": "30 days",

    // Metric Detail Page
    "metric_detail.last_days": "Last %1 days",
    "metric_detail.not_enough_history": "Not enough history",
    "metric_detail.not_enough_hint": "One data point cannot make a trend. Keep wearing the watch and sync daily.",
    "metric_detail.min": "Min",
    "metric_detail.avg": "Average",
    "metric_detail.max": "Max",
    "metric_detail.samples": "Samples",
    "metric_detail.no_samples": "No samples in this range.",

    // Fitness Page
    "fitness.kicker": "Activities",
    "fitness.title": "Fitness",
    "fitness.all_workouts": "All workouts",
    "fitness.no_workouts_title": "No workouts recorded",
    "fitness.no_workouts_hint": "Start an activity on the watch. After the next sync it shows up here with its full trace.",
    "fitness.summary_this_week": "This week",

    // Workout Detail Page
    "workout_detail.route": "Route",
    "workout_detail.summary": "Summary",
    "workout_detail.traces": "Traces",
    "workout_detail.pace": "Pace",
    "workout_detail.speed": "Speed",
    "workout_detail.altitude": "Altitude",
    "workout_detail.cadence": "Cadence",
    "workout_detail.power": "Power",
    "workout_detail.failed_title": "Could not load this workout",
    "workout_detail.no_trace_title": "No detailed trace",
    "workout_detail.no_trace_hint": "This activity was stored as a summary only — the watch did not send per-second samples.",

    // Device Page
    "device.title": "Device",
    "device.transferring": "Transferring files",
    "device.files_progress": "File %1 · %2 of %3 bytes · %4 files left",
    "device.paired_section": "Paired",
    "device.no_watch_paired": "No watch paired yet",
    "device.pair_hint": "Put the Garmin in pairing mode, then scan below.",
    "device.daemon_offline_hint": "Start pulsed — Bluetooth is handled by the daemon, not this app.",
    "device.nearby_section": "Nearby",
    "device.scanning": "Scanning…",
    "device.nothing_found": "Nothing found yet",
    "device.keep_awake_hint": "Keep the watch awake and close to the phone.",
    "device.scan_hint": "Tap Scan and hold the watch nearby. Garmin devices are flagged automatically.",
    "device.appearance_section": "Appearance",
    "device.theme": "Theme",
    "device.theme_system": "System",
    "device.theme_light": "Light",
    "device.theme_dark": "Dark",
    "device.accent": "Accent",
    "device.units_section": "Units",
    "device.units": "Units",
    "device.units_metric": "Metric",
    "device.units_imperial": "Imperial",
    "device.goals_section": "Goals",
    "device.goal_steps": "Steps",
    "device.goal_sleep": "Sleep",
    "device.goal_calories": "Active calories",
    "device.goal_distance": "Distance",
    "device.goal_active_mins": "Active minutes",
    "device.goal_intensity": "Intensity minutes",
    "device.integrations_section": "Watch behaviour",
    "device.sync_time_title": "Sync time",
    "device.sync_time_sub": "Set the watch clock on connect",
    "device.weather_title": "Weather",
    "device.weather_sub": "Answer the watch's weather requests",
    "device.notifications_title": "Forward notifications",
    "device.notifications_sub": "Send desktop notifications to the watch",
    "device.waydroid_title": "Include Waydroid apps",
    "device.waydroid_sub": "Also forward Android notifications from the container",
    "device.keep_files_title": "Keep files on watch",
    "device.keep_files_sub": "Do not delete FIT files after download",

    // Notifications Page
    "notifications.kicker": "Sent to the watch",
    "notifications.title": "Notifications",
    "notifications.src_desktop": "Desktop",
    "notifications.src_waydroid": "Waydroid",
    "notifications.src_call": "Calls",
    "notifications.empty_title": "Nothing forwarded yet",
    "notifications.empty_hint": "Notifications appear here the moment pulsed relays one to the watch. Check that forwarding is enabled on the Device tab.",

    // Pairing Sheet
    "pairing.enter_code": "Enter the code shown on the watch",
    "pairing.confirm": "Confirm pairing",

    // Accents
    "accent.blue": "Neon Blue",
    "accent.violet": "Violet",
    "accent.coral": "Coral",
    "accent.mint": "Mint",
    "accent.pink": "Pink",

    // Toast messages
    "toast.settings_saved": "Settings saved",
    "toast.settings_save_failed": "Could not save settings",
    "toast.scan_failed": "Scan failed: %1",
    "toast.pairing_failed": "Pairing failed: %1",
    "toast.pairing_reply_failed": "Pairing reply failed",
    "toast.connect_failed": "Connect failed",
    "toast.disconnect_failed": "Disconnect failed",
    "toast.forget_failed": "Could not forget device",
    "toast.sync_failed": "Sync failed",
    "toast.watch_ringing": "Watch is ringing",
    "toast.find_watch_failed": "Find my watch failed",

    // Device rows
    "device.row_connected": "connected",
    "device.row_offline": "offline",
    "device.footer_online": "Connected to pulsed %1 on 127.0.0.1:21830",
    "device.footer_offline": "pulsed is not answering on 127.0.0.1:21830",

    // Workout summary keys (free-form JSON from the daemon)
    "sum.distance": "Distance",
    "sum.duration": "Duration",
    "sum.movingTime": "Moving time",
    "sum.elapsedTime": "Elapsed time",
    "sum.calories": "Calories",
    "sum.activeCalories": "Active calories",
    "sum.avgHeartRate": "Avg heart rate",
    "sum.maxHeartRate": "Max heart rate",
    "sum.minHeartRate": "Min heart rate",
    "sum.avgSpeed": "Avg speed",
    "sum.maxSpeed": "Max speed",
    "sum.avgPace": "Avg pace",
    "sum.avgCadence": "Avg cadence",
    "sum.maxCadence": "Max cadence",
    "sum.avgPower": "Avg power",
    "sum.maxPower": "Max power",
    "sum.normalizedPower": "Normalized power",
    "sum.ascent": "Ascent",
    "sum.descent": "Descent",
    "sum.totalAscent": "Total ascent",
    "sum.totalDescent": "Total descent",
    "sum.maxAltitude": "Max altitude",
    "sum.minAltitude": "Min altitude",
    "sum.avgTemperature": "Avg temperature",
    "sum.maxTemperature": "Max temperature",
    "sum.steps": "Steps",
    "sum.strokes": "Strokes",
    "sum.laps": "Laps",
    "sum.pool": "Pool length",
    "sum.swolf": "SWOLF",
    "sum.trainingEffect": "Training effect",
    "sum.anaerobicTrainingEffect": "Anaerobic effect",
    "sum.aerobicTrainingEffect": "Aerobic effect",
    "sum.vo2Max": "VO2 max",
    "sum.recoveryTime": "Recovery time",
    "sum.avgRespirationRate": "Avg respiration",
    "sum.startTime": "Start",
    "sum.endTime": "End",
    "sum.sport": "Sport",
    "sum.subSport": "Sub-sport",

    // Units
    "unit.steps": "steps",
    "unit.bpm": "bpm",
    "unit.kcal": "kcal",
    "unit.km": "km",
    "unit.m": "m",
    "unit.mi": "mi",
    "unit.min": "min",
    "unit.h": "h",
    "unit.spm": "spm",
    "unit.w": "W",
    "unit.pct": "%",
    "unit.brpm": "brpm",
    "unit.ms": "ms",

    // Sports & Activities
    "sport.generic": "Generic",
    "sport.run": "Run",
    "sport.ride": "Ride",
    "sport.swim": "Swim",
    "sport.basketball": "Basketball",
    "sport.soccer": "Soccer",
    "sport.tennis": "Tennis",
    "sport.row": "Row",
    "sport.walk": "Walk",
    "sport.alpine_ski": "Alpine ski",
    "sport.hike": "Hike",
    "sport.multisport": "Multisport",
    "sport.strength": "Strength",
    "sport.cardio": "Cardio",
    "sport.paddling": "Paddling",
    "sport.yoga": "Yoga",
    "sport.treadmill": "Treadmill",
    "sport.exercise": "Exercise",
    "sport.activity": "Activity",
    "sport.navigate": "Navigate",
    "sport.indoor_track": "Indoor track run",
    "sport.workout": "Workout",

    // Date / Time
    "date.today": "Today",
    "date.yesterday": "Yesterday",
    "date.never": "never",
    "date.just_now": "just now",
    "date.mins_ago": "%1 min ago",
    "date.hours_ago": "%1h ago",
    "date.days_ago": "%1d ago",

    // Greetings
    "greeting.morning": "Good morning",
    "greeting.afternoon": "Good afternoon",
    "greeting.evening": "Good evening"
};

var DICT_RU = {
    // Tabs
    "tab.today": "Сегодня",
    "tab.health": "Здоровье",
    "tab.sleep": "Сон",
    "tab.fitness": "Тренировки",
    "tab.device": "Устройство",

    // Common actions & buttons
    "action.connect": "Подключить",
    "action.disconnect": "Отключить",
    "action.sync": "Синхронизация",
    "action.syncing": "Синхронизация…",
    "action.sync_now": "Синхронизировать",
    "action.find_watch": "Найти часы",
    "action.stop_ringing": "Остановить звонок",
    "action.scan": "Поиск",
    "action.stop": "Стоп",
    "action.pair": "Подключить",
    "action.paired": "Подключено",
    "action.cancel": "Отмена",
    "action.confirm": "Подтвердить",
    "action.retry": "Повторить",
    "action.see_all": "Все",
    "action.dismiss": "Закрыть",
    "action.steady": "без изменений",

    // Status strip
    "status.daemon_offline": "Демон Pulse не запущен",
    "status.start_daemon": "Запустите pulsed для просмотра данных",
    "status.no_watch": "Часы не сопряжены",
    "status.open_device_scan": "Откройте «Устройство» для поиска и сопряжения",
    "status.connected": "Подключено",
    "status.connecting": "Подключение…",
    "status.disconnected": "Отключено",
    "status.synced_relative": "синхр. %1",
    "status.syncing_file": "Синхронизация файла %1 · %2%",
    "status.syncing_ellipsis": "Синхронизация…",

    // Today Page
    "today.steps": "ШАГИ",
    "today.reading": "Загрузка…",
    "today.no_steps": "Сегодня шагов пока нет.",
    "today.goal_beaten": "Цель превышена на %1 шагов.",
    "today.steps_to_go": "Осталось %1 шагов до цели.",
    "today.recent_workouts": "Недавние тренировки",
    "today.no_workouts_title": "Тренировок пока нет",
    "today.no_workouts_hint": "Запишите тренировку на часах и синхронизируйте их.",
    "today.daemon_wait_title": "Ожидание демона",
    "today.daemon_unreachable": "Приложение не может подключиться к pulsed на 127.0.0.1:21830.",

    // Metrics & Tiles
    "metric.steps": "Шаги",
    "metric.distance": "Дистанция",
    "metric.activetime": "Активное время",
    "metric.active_minutes": "Активные минуты",
    "metric.calories": "Калории",
    "metric.intensity": "Интенсивность",
    "metric.sleep": "Сон",
    "metric.heart_rate": "Пульс",
    "metric.resting_hr": "Пульс покоя",
    "metric.body_energy": "Body Battery",
    "metric.stress": "Стресс",
    "metric.spo2": "Кислород в крови",
    "metric.hrv": "ВСР",
    "metric.respiration": "Дыхание",

    // Sleep Page
    "sleep.title": "Сон",
    "sleep.no_night_title": "Нет данных за ночь",
    "sleep.no_night_hint": "Не снимайте часы во время сна, затем синхронизируйте. Ночи учитываются по дате пробуждения.",
    "sleep.start_pulsed_hint": "Запустите pulsed на 127.0.0.1:21830 для загрузки данных сна.",
    "sleep.score_label": "Качество сна",
    "sleep.goal_share": "от цели",
    "sleep.hypnogram": "Гипнограмма",
    "sleep.no_stages": "Детализация стадий отсутствует",
    "sleep.last_7_nights": "Последние 7 ночей",
    "sleep.naps": "Дневной сон",
    "sleep.avg_nights": "В среднем %1 за %2",
    "sleep.no_nights_yet": "Ночей пока не записано",
    "sleep.deep": "Глубокий",
    "sleep.light": "Лёгкий",
    "sleep.rem": "Быстрый (REM)",
    "sleep.awake": "Бодрствование",
    "sleep.quality_excellent": "Отлично",
    "sleep.quality_good": "Хорошо",
    "sleep.quality_fair": "Нормально",
    "sleep.quality_poor": "Плохо",

    // Health Page
    "health.kicker": "Динамика",
    "health.title": "Здоровье",
    "health.no_history_title": "Нет истории показателей",
    "health.no_history_hint": "Носите часы в течение дня и синхронизируйте — пульс, Body Battery, стресс, SpO₂, ВСР и дыхание появятся здесь.",
    "health.next_sync_hint": "Запустите pulsed, и данные появятся при следующей синхронизации.",
    "health.deltas_hint": "Дельты показывают отличие последнего значения от среднего за предыдущие дни.",
    "health.days_7": "7 дней",
    "health.days_14": "14 дней",
    "health.days_30": "30 дней",

    // Metric Detail Page
    "metric_detail.last_days": "За %1 дн.",
    "metric_detail.not_enough_history": "Недостаточно данных",
    "metric_detail.not_enough_hint": "Одно значение не показывает тренд. Носите часы и синхронизируйте ежедневно.",
    "metric_detail.min": "Мин",
    "metric_detail.avg": "Среднее",
    "metric_detail.max": "Макс",
    "metric_detail.samples": "Замеры",
    "metric_detail.no_samples": "Нет замеров за этот период.",

    // Fitness Page
    "fitness.kicker": "Активность",
    "fitness.title": "Тренировки",
    "fitness.all_workouts": "Все тренировки",
    "fitness.no_workouts_title": "Тренировки не записаны",
    "fitness.no_workouts_hint": "Запустите тренировку на часах. После синхронизации здесь появится подробный трек.",
    "fitness.summary_this_week": "На этой неделе",

    // Workout Detail Page
    "workout_detail.route": "Маршрут",
    "workout_detail.summary": "Сводка",
    "workout_detail.traces": "Графики",
    "workout_detail.pace": "Темп",
    "workout_detail.speed": "Скорость",
    "workout_detail.altitude": "Высота",
    "workout_detail.cadence": "Каденс",
    "workout_detail.power": "Мощность",
    "workout_detail.failed_title": "Не удалось загрузить тренировку",
    "workout_detail.no_trace_title": "Детальный трек отсутствует",
    "workout_detail.no_trace_hint": "Тренировка сохранена только со сводными данными — посекундные замеры отсутствуют.",

    // Device Page
    "device.title": "Устройство",
    "device.transferring": "Передача файлов",
    "device.files_progress": "Файл %1 · %2 из %3 байт · осталось файлов: %4",
    "device.paired_section": "Сопряжённые устройства",
    "device.no_watch_paired": "Часы пока не сопряжены",
    "device.pair_hint": "Переведите Garmin в режим сопряжения и запустите поиск ниже.",
    "device.daemon_offline_hint": "Запустите pulsed — Bluetooth управляется демоном, а не интерфейсом.",
    "device.nearby_section": "Устройства рядом",
    "device.scanning": "Поиск…",
    "device.nothing_found": "Устройств пока не найдено",
    "device.keep_awake_hint": "Держите часы активными рядом с телефоном.",
    "device.scan_hint": "Нажмите «Поиск» рядом с часами. Устройства Garmin определяются автоматически.",
    "device.appearance_section": "Внешний вид",
    "device.theme": "Тема",
    "device.theme_system": "Системная",
    "device.theme_light": "Светлая",
    "device.theme_dark": "Тёмная",
    "device.accent": "Акцентный цвет",
    "device.units_section": "Единицы измерения",
    "device.units": "Единицы",
    "device.units_metric": "Метрические (км, м)",
    "device.units_imperial": "Британские (мили, футы)",
    "device.goals_section": "Дневные цели",
    "device.goal_steps": "Шаги",
    "device.goal_sleep": "Сон (минуты)",
    "device.goal_calories": "Активные калории (ккал)",
    "device.goal_distance": "Дистанция (метры)",
    "device.goal_active_mins": "Активные минуты",
    "device.goal_intensity": "Интенсивность",
    "device.integrations_section": "Синхронизация и интеграции",
    "device.sync_time_title": "Синхронизация времени",
    "device.sync_time_sub": "Корректировать часы по времени телефона при подключении",
    "device.weather_title": "Прогноз погоды",
    "device.weather_sub": "Отправлять погоду Open-Meteo на часы",
    "device.notifications_title": "Уведомления",
    "device.notifications_sub": "Пересылать системные уведомления Ubuntu Touch на часы",
    "device.waydroid_title": "Уведомления Waydroid",
    "device.waydroid_sub": "Пересылать уведомления Android-приложений из контейнера",
    "device.keep_files_title": "Хранить файлы на часах",
    "device.keep_files_sub": "Не помечать скачанные FIT-файлы как заархивированные",

    // Notifications Page
    "notifications.kicker": "Отправлено на часы",
    "notifications.title": "Уведомления",
    "notifications.src_desktop": "Система",
    "notifications.src_waydroid": "Waydroid",
    "notifications.src_call": "Звонки",
    "notifications.empty_title": "Уведомлений пока не было",
    "notifications.empty_hint": "Уведомления появятся здесь, как только pulsed перешлёт их на часы. Проверьте включение пересылки во вкладке «Устройство».",

    // Pairing Sheet
    "pairing.enter_code": "Введите код, показанный на часах",
    "pairing.confirm": "Подтверждение сопряжения",

    // Accents
    "accent.blue": "Неоновый синий",
    "accent.violet": "Фиолетовый",
    "accent.coral": "Коралловый",
    "accent.mint": "Мятный",
    "accent.pink": "Розовый",

    // Toast messages
    "toast.settings_saved": "Настройки сохранены",
    "toast.settings_save_failed": "Не удалось сохранить настройки",
    "toast.scan_failed": "Ошибка поиска: %1",
    "toast.pairing_failed": "Ошибка сопряжения: %1",
    "toast.pairing_reply_failed": "Не удалось отправить ответ сопряжения",
    "toast.connect_failed": "Не удалось подключиться к часам",
    "toast.disconnect_failed": "Не удалось отключить часы",
    "toast.forget_failed": "Не удалось удалить устройство",
    "toast.sync_failed": "Ошибка синхронизации",
    "toast.watch_ringing": "Часы подают сигнал",
    "toast.find_watch_failed": "Не удалось запустить поиск часов",

    // Device rows
    "device.row_connected": "подключено",
    "device.row_offline": "не в сети",
    "device.footer_online": "Подключено к pulsed %1 на 127.0.0.1:21830",
    "device.footer_offline": "pulsed не отвечает на 127.0.0.1:21830",

    // Workout summary keys (free-form JSON from the daemon)
    "sum.distance": "Дистанция",
    "sum.duration": "Длительность",
    "sum.movingTime": "Время в движении",
    "sum.elapsedTime": "Общее время",
    "sum.calories": "Калории",
    "sum.activeCalories": "Активные калории",
    "sum.avgHeartRate": "Средний пульс",
    "sum.maxHeartRate": "Максимальный пульс",
    "sum.minHeartRate": "Минимальный пульс",
    "sum.avgSpeed": "Средняя скорость",
    "sum.maxSpeed": "Максимальная скорость",
    "sum.avgPace": "Средний темп",
    "sum.avgCadence": "Средний каденс",
    "sum.maxCadence": "Максимальный каденс",
    "sum.avgPower": "Средняя мощность",
    "sum.maxPower": "Максимальная мощность",
    "sum.normalizedPower": "Нормализованная мощность",
    "sum.ascent": "Набор высоты",
    "sum.descent": "Спуск",
    "sum.totalAscent": "Суммарный набор",
    "sum.totalDescent": "Суммарный спуск",
    "sum.maxAltitude": "Максимальная высота",
    "sum.minAltitude": "Минимальная высота",
    "sum.avgTemperature": "Средняя температура",
    "sum.maxTemperature": "Максимальная температура",
    "sum.steps": "Шаги",
    "sum.strokes": "Гребки",
    "sum.laps": "Отрезки",
    "sum.pool": "Длина бассейна",
    "sum.swolf": "SWOLF",
    "sum.trainingEffect": "Тренировочный эффект",
    "sum.anaerobicTrainingEffect": "Анаэробный эффект",
    "sum.aerobicTrainingEffect": "Аэробный эффект",
    "sum.vo2Max": "МПК (VO2 max)",
    "sum.recoveryTime": "Время восстановления",
    "sum.avgRespirationRate": "Среднее дыхание",
    "sum.startTime": "Начало",
    "sum.endTime": "Конец",
    "sum.sport": "Вид спорта",
    "sum.subSport": "Подвид",

    // Units
    "unit.steps": "шагов",
    "unit.bpm": "уд/мин",
    "unit.kcal": "ккал",
    "unit.km": "км",
    "unit.m": "м",
    "unit.mi": "ми",
    "unit.min": "мин",
    "unit.h": "ч",
    "unit.spm": "шаг/мин",
    "unit.w": "Вт",
    "unit.pct": "%",
    "unit.brpm": "вдох/мин",
    "unit.ms": "мс",

    // Sports & Activities
    "sport.generic": "Тренировка",
    "sport.run": "Бег",
    "sport.ride": "Велосипед",
    "sport.swim": "Плавание",
    "sport.basketball": "Баскетбол",
    "sport.soccer": "Футбол",
    "sport.tennis": "Теннис",
    "sport.row": "Гребля",
    "sport.walk": "Ходьба",
    "sport.alpine_ski": "Горные лыжи",
    "sport.hike": "Хайкинг",
    "sport.multisport": "Мультиспорт",
    "sport.strength": "Силовая",
    "sport.cardio": "Кардио",
    "sport.paddling": "Байдарка/SUP",
    "sport.yoga": "Йога",
    "sport.treadmill": "Беговая дорожка",
    "sport.exercise": "Упражнения",
    "sport.activity": "Активность",
    "sport.navigate": "Навигация",
    "sport.indoor_track": "Бег в манеже",
    "sport.workout": "Тренировка",

    // Date / Time
    "date.today": "Сегодня",
    "date.yesterday": "Вчера",
    "date.never": "никогда",
    "date.just_now": "только что",
    "date.mins_ago": "%1 мин назад",
    "date.hours_ago": "%1 ч назад",
    "date.days_ago": "%1 дн назад",

    // Greetings
    "greeting.morning": "Доброе утро",
    "greeting.afternoon": "Добрый день",
    "greeting.evening": "Добрый вечер"
};

// Detect system language from Qt environment.
function detect() {
    try {
        var loc = Qt.locale().name;
        if (loc && (loc.indexOf("ru") === 0 || loc.indexOf("RU") === 0)) {
            currentLang = "ru";
            return "ru";
        }
    } catch (e) {}

    // Check if Qt uiLanguages has Russian
    try {
        var uiLangs = Qt.locale().uiLanguages;
        if (uiLangs && uiLangs.length > 0) {
            for (var i = 0; i < uiLangs.length; i++) {
                if (uiLangs[i].indexOf("ru") === 0) {
                    currentLang = "ru";
                    return "ru";
                }
            }
        }
    } catch (e) {}

    currentLang = "en";
    return "en";
}

function current() {
    return currentLang;
}

function setLang(l) {
    if (l === "ru" || l === "en") {
        currentLang = l;
    }
}

function isRu() {
    return currentLang === "ru";
}

// Translate key.
// If current language is Russian: lookup in DICT_RU.
// If current language is English: lookup in DICT_EN, fallback to key.
// Supports replacing %1, %2, etc. with arguments if provided.
function t(key, args) {
    var val = key;
    if (currentLang === "ru") {
        if (DICT_RU.hasOwnProperty(key)) {
            val = DICT_RU[key];
        } else if (DICT_EN.hasOwnProperty(key)) {
            val = DICT_EN[key];
        }
    } else {
        if (DICT_EN.hasOwnProperty(key)) {
            val = DICT_EN[key];
        }
    }

    if (args !== undefined && args !== null) {
        if (!Array.isArray(args)) {
            args = [args];
        }
        for (var i = 0; i < args.length; i++) {
            val = val.replace("%" + (i + 1), args[i]);
        }
    }
    return val;
}

// Alias for convenience
function tr(key, args) {
    return t(key, args);
}
