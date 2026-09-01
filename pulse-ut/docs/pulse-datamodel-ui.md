# Pulse: модель данных (БД) + UI-спецификация для порта на Ubuntu Touch (Go + QML)

Все ссылки — на /Users/qq/garpun-ut/pulse-main.
Схема БД генерируется из GBDaoGenerator/src/nodomain/freeyourgadget/gadgetbridge/daogen/GBDaoGenerator.java (Schema 138, package nodomain.freeyourgadget.gadgetbridge.entities — GBDaoGenerator.java:118).

---

## 1. Схема БД (только то, что реально заполняет Garmin-стек)

### 1.1 Общие правила генератора

* addCommonActivitySampleProperties(...) (GBDaoGenerator.java:1510-1524) — «activity sample»:
  * timestamp INTEGER NOT NULL — СЕКУНДЫ Unix, PK
  * deviceId INTEGER NOT NULL — PK (составной с timestamp)
  * userId INTEGER NOT NULL
* addCommonTimeSampleProperties(...) (GBDaoGenerator.java:1526-1537) — «time sample»:
  * timestamp INTEGER NOT NULL — МИЛЛИСЕКУНДЫ Unix, PK
  * deviceId INTEGER NOT NULL — PK
  * userId INTEGER NOT NULL
* Имена таблиц greenDAO = UPPER_SNAKE от сущности (GarminActivitySample -> GARMIN_ACTIVITY_SAMPLE), колонки — UPPER_SNAKE от свойств (rawKind -> RAW_KIND).

КРИТИЧНО: activity-samples в секундах, все time-samples (стресс, body energy, spo2, hrv, стадии сна и т.д.) — в миллисекундах. selectedDayStart()/End() отдают ms (DashboardFragment.java:702-720), dashboardData.timeFrom/timeTo — секунды (DashboardFragment.java:784-786).

### 1.2 Базовые сущности

```sql
-- GBDaoGenerator.java:465-489
USER(_id PK, name TEXT NOT NULL, birthday INTEGER(date) NOT NULL, gender INTEGER NOT NULL)

-- GBDaoGenerator.java:490-511 (история атрибутов, сортировка по validFromUTC DESC)
USER_ATTRIBUTES(_id PK, userId INTEGER NOT NULL,
                heightCM INTEGER NOT NULL, weightKG INTEGER NOT NULL,
                sleepGoalHPD INTEGER /*deprecated*/, stepsGoalSPD INTEGER,
                validFromUTC INTEGER(date), validToUTC INTEGER(date),
                sleepGoalMPD INTEGER)

-- GBDaoGenerator.java:513-531
DEVICE(_id PK, name TEXT NOT NULL, manufacturer TEXT NOT NULL,
       identifier TEXT NOT NULL UNIQUE /*MAC*/, type INTEGER NOT NULL /*deprecated*/,
       typeName TEXT NOT NULL, model TEXT, alias TEXT, parentFolder TEXT)

-- GBDaoGenerator.java:533-541 + :139 (volatileIdentifier добавляется после addDevice)
DEVICE_ATTRIBUTES(_id PK, deviceId INTEGER NOT NULL,
                  firmwareVersion1 TEXT NOT NULL, firmwareVersion2 TEXT,
                  validFromUTC INTEGER(date), validToUTC INTEGER(date),
                  volatileIdentifier TEXT)
```

### 1.3 Garmin-специфичные таблицы

```sql
-- GarminActivitySample (GBDaoGenerator.java:1142-1154) — минутные семплы, ts в СЕКУНДАХ
GARMIN_ACTIVITY_SAMPLE(timestamp INT PK, deviceId INT PK, userId INT NOT NULL,
    rawIntensity INT NOT NULL, steps INT NOT NULL, rawKind INT NOT NULL,
    heartRate INT NOT NULL, distanceCm INT NOT NULL, activeCalories INT NOT NULL)
    -- rawKind = код ActivityKind (§1.5); activeCalories «сырые» (÷1000 = ккал)

-- ниже везде ts в МИЛЛИСЕКУНДАХ
GARMIN_STRESS_SAMPLE(timestamp, deviceId, userId, stress INT NOT NULL)              -- :1156
GARMIN_BODY_ENERGY_SAMPLE(..., energy INT NOT NULL)                                 -- :1163 (Body Battery 0..100)
GARMIN_SPO2_SAMPLE(..., spo2 INT NOT NULL, typeNum INT NOT NULL)                    -- :1170
GARMIN_SLEEP_STAGE_SAMPLE(..., stage INT NOT NULL)                                  -- :1178
GARMIN_EVENT_SAMPLE(..., event INT NOT NULL PK, eventType INT, data INT)            -- :1185
GARMIN_HRV_SUMMARY_SAMPLE(..., weeklyAverage, lastNightAverage, lastNight5MinHigh,
    baselineLowUpper, baselineBalancedLower, baselineBalancedUpper, statusNum)       -- :1194
GARMIN_HRV_VALUE_SAMPLE(..., value INT NOT NULL)                                    -- :1207 (мс)
GARMIN_RESPIRATORY_RATE_SAMPLE(..., respiratoryRate REAL NOT NULL)                  -- :1214
GARMIN_HEART_RATE_RESTING_SAMPLE(..., heartRate INT NOT NULL)                       -- :1221
GARMIN_RESTING_METABOLIC_RATE_SAMPLE(..., restingMetabolicRate INT NOT NULL)        -- :1228 (ккал/сутки)
GARMIN_SLEEP_STATS_SAMPLE(..., sleepScore INT NOT NULL)                             -- :1236
GARMIN_INTENSITY_MINUTES_SAMPLE(..., moderate INT, vigorous INT)                    -- :1243
GARMIN_NAP_SAMPLE(..., endTimestamp INT NOT NULL)                                   -- :1251
GARMIN_SLEEP_RESTLESS_MOMENTS_SAMPLE(..., count INT NOT NULL)                       -- :1258

-- сырые FIT-файлы (GBDaoGenerator.java:1113-1140)
GARMIN_FIT_FILE(_id PK AUTOINCREMENT, downloadTimestamp INT NOT NULL,
    deviceId INT NOT NULL, userId INT NOT NULL, fileNumber INT NOT NULL,
    fileDataType INT NOT NULL, fileSubType INT NOT NULL, fileTimestamp INT NOT NULL,
    specificFlags INT NOT NULL, fileSize INT NOT NULL, fileData BLOB,
    UNIQUE(deviceId, userId, fileNumber))

-- очередь необработанных файлов (GBDaoGenerator.java:1265-1286)
PENDING_FILE(_id PK AUTOINCREMENT, path TEXT NOT NULL, deviceId INT NOT NULL,
    UNIQUE(deviceId, path))
```

### 1.4 Generic-таблицы, которые заполняет именно Garmin

FitImporter (service/devices/garmin/fit/FitImporter.java:141-165, 577-608) пишет: GarminStress/BodyEnergy/Spo2/RespiratoryRate/HeartRateResting/RestingMetabolicRate (MONITOR), GarminEvent/SleepStats/Nap/SleepRestlessMoments/SleepStage (SLEEP), GarminHrvSummary/HrvValue (HRV_STATUS), GarminActivitySample + GarminIntensityMinutes, BatteryLevel, а также:

```sql
-- GBDaoGenerator.java:2524-2536
GENERIC_TRAINING_LOAD_ACUTE_SAMPLE(timestamp ms, deviceId, userId, value INT NOT NULL)
GENERIC_TRAINING_LOAD_CHRONIC_SAMPLE(timestamp ms, deviceId, userId, value INT NOT NULL)

-- GBDaoGenerator.java:2563-2571 — универсальные метрики (VO2max, endurance, readiness, …)
GENERIC_METRIC_SAMPLE(timestamp ms PK, deviceId PK, userId,
    metricType INT NOT NULL PK, metricScore REAL NOT NULL, metricExtra INT)
```

metricType — enum MetricSample.Metric (model/MetricSample.java:77-132):
0 UNKNOWN, 1 GARMIN_ENDURANCE_SCORE, 2 GARMIN_FUNCTIONAL_THRESHOLD_POWER, 3 GARMIN_HILL_ENDURANCE, 4 GARMIN_HILL_SCORE, 5 GARMIN_HILL_STRENGTH, 6 GARMIN_MET_MAX_VO2, 7 GARMIN_RUNNING_LACTATE_THRESHOLD_POWER, 8 GARMIN_TRAINING_READINESS, 9 GENERIC_TRAINING_LOAD_ACUTE, 10 GENERIC_TRAINING_LOAD_CHRONIC, 11 GENERIC_RESTING_METABOLIC_RATE, 12 GENERIC_MAXIMUM_OXYGEN_UPTAKE, 13 GENERIC_SLEEP_SCORE, 14 GENERIC_READINESS, 15 GENERIC_ENERGY, 16 GENERIC_BODY_BATTERY, 17 GENERIC_CARDIAC_STRAIN, 18 GENERIC_SLEEP_REGULARITY.

### 1.5 Тренировки, батарея, коды активности

```sql
-- BaseActivitySummary (GBDaoGenerator.java:1744-1771)
BASE_ACTIVITY_SUMMARY(_id PK, name TEXT, startTime INTEGER(date) NOT NULL,
    endTime INTEGER(date) NOT NULL, activityKind INT NOT NULL,
    baseLongitude INT, baseLatitude INT, baseAltitude INT,
    gpxTrack TEXT, rawDetailsPath TEXT, headerPhoto TEXT,
    deviceId INT NOT NULL, userId INT NOT NULL,
    summaryData TEXT /*JSON*/, rawSummaryData BLOB)

-- BatteryLevel (GBDaoGenerator.java:1788-1797) — timestamp в СЕКУНДАХ (FIXME на :275)
BATTERY_LEVEL(timestamp INT PK, deviceId INT PK, level INT NOT NULL, batteryIndex INT PK)
```

activityKind — коды ActivityKind (model/ActivityKind.java:28-56 и далее до :232): NOT_MEASURED=-1, UNKNOWN=0x0, ACTIVITY=0x1, LIGHT_SLEEP=0x2, DEEP_SLEEP=0x4, NOT_WORN=0x8, RUNNING=0x10, WALKING=0x20, SWIMMING=0x40, CYCLING=0x80, TREADMILL=0x100, EXERCISE=0x200, …, REM_SLEEP=0x01000000, AWAKE_SLEEP=0x02000000, SLEEP_ANY=0x2|0x4|0x01000000|0x02000000; «новые» виды спорта — с 0x04000000 (NAVIGATE=0x04000000, INDOOR_TRACK_RUNNING=0x04000001, …).

Стадии сна Garmin: GarminSleepStageSample.stage — FIT sleep_level; маппинг в devices/garmin/GarminActivitySampleProvider.java:236-253: AWAKE->AWAKE_SLEEP, LIGHT->LIGHT_SLEEP, DEEP->DEEP_SLEEP, REM->REM_SLEEP, иначе UNKNOWN. Комментарий FitImporter.java:596 фиксирует «0 unmeasurable, 1 awake» (enum SleepStage генерируется FIT-кодогеном, в src его нет).
События сна: GarminEventSample.event=74 («sleep»), eventType=0 — засыпание, 1 — пробуждение (FitImporter.java:926-937). GarminActivitySampleProvider использует их, чтобы дотянуть стадию за границу диапазона (:169-194).

### 1.6 Минимальный набор таблиц для Go-порта

Обязательно: USER, USER_ATTRIBUTES, DEVICE, DEVICE_ATTRIBUTES, GARMIN_ACTIVITY_SAMPLE, GARMIN_SLEEP_STAGE_SAMPLE, GARMIN_EVENT_SAMPLE, GARMIN_SLEEP_STATS_SAMPLE, GARMIN_NAP_SAMPLE, GARMIN_SLEEP_RESTLESS_MOMENTS_SAMPLE, GARMIN_STRESS_SAMPLE, GARMIN_BODY_ENERGY_SAMPLE, GARMIN_SPO2_SAMPLE, GARMIN_HRV_VALUE_SAMPLE, GARMIN_HRV_SUMMARY_SAMPLE, GARMIN_RESPIRATORY_RATE_SAMPLE, GARMIN_HEART_RATE_RESTING_SAMPLE, GARMIN_RESTING_METABOLIC_RATE_SAMPLE, GARMIN_INTENSITY_MINUTES_SAMPLE, BASE_ACTIVITY_SUMMARY, BATTERY_LEVEL, GARMIN_FIT_FILE, PENDING_FILE, GENERIC_METRIC_SAMPLE, GENERIC_TRAINING_LOAD_ACUTE_SAMPLE, GENERIC_TRAINING_LOAD_CHRONIC_SAMPLE.
Не нужны: все таблицы Huami/Xiaomi/Huawei/Colmi/Moyoung/Pebble/Wena/Lefun/… (в Garmin-only форке не заполняются).

---

## 2. Агрегаты и формулы

### 2.1 DailyTotals (model/DailyTotals.java)

* getDailyTotalsForDevice(device, day, db) (DailyTotals.java:92-115):
  * шаги/дистанция/активные калории — сумма по семплам окна [00:00 дня, +24h−1s) (getSamplesOfDay(..., offsetHours=0), :139-153);
  * сон — то же окно, но со сдвигом −12 часов (offsetHours=-12), т.е. ночь принадлежит дню пробуждения;
  * restingCalories = restingMetabolicRate × доля прошедшего дня (:155-181); для текущего дня доля = (now − 00:00)/86400000, иначе 1. RMR берётся из GARMIN_RESTING_METABOLIC_RATE_SAMPLE (последний до конца дня) либо из DefaultRestingMetabolicRateProvider.
* getSleep() = light + deep + rem в МИНУТАХ, awake исключён (:78-81, 117-137).
* Внутреннее представление sleep[] = {light, deep, rem, awake} минут.

### 2.2 Цели пользователя (model/ActivityUser.java:52-82)

| Метрика | Ключ | Дефолт |
|---|---|---|
| Шаги | fitness_goal | defaultUserStepsGoal = 8000 (в pulse_goals.xml пресет 10000) |
| Сон, мин | activity_user_sleep_duration_minutes | 7*60 = 420 (пикер PulseGoals fallback 480, PulseGoalsActivity.java:82) |
| Активные минуты | activity_user_activetime_minutes | 60 |
| Активные ккал | activity_user_calories_burnt | 350 |
| Дистанция, м | activity_user_distance_meters | 5000 |
| Стояние, ч | activity_user_goal_standing_hours | 12 |
| Fat-burn, мин | activity_user_goal_fat_burn_time_minutes | 30 |
| Целевой вес, кг | activity_user_goal_weight_kg | 70 |
| Intensity minutes/день | pulse_intensity_goal | 30 (pulse_goals.xml; DashboardFragment.java:978) |
| Рост / вес | activity_user_height_cm = 175 / activity_user_weight_kg = 70 | |
| Длина шага | activity_user_step_length_cm; 0 => height × 0.43 (ActivityUser.java:120-126) | |
| Имя | mi_user_alias | gadgetbridge-user |

Санитайзеры: sleepDurationGoal вне (0, 1440] => дефолт; остальные < 1 => дефолт. Пол: GENDER_FEMALE=0, GENDER_MALE=1, GENDER_OTHER=2 (дефолт FEMALE), дата рождения по умолчанию 2000-01-01.

### 2.3 Goal-факторы (util/DashboardUtils.java) — все клампятся единицей

* stepsGoalFactor = min(1, steps / stepsGoal) (:104-111)
* sleepMinutesGoalFactor = min(1, sleepMinutes / sleepGoal) (:135-142)
* distanceGoalFactor = min(1, distanceMeters / distanceGoalMeters) (:166-173)
* activeCaloriesGoalFactor = min(1, kcal / caloriesGoal) (:175-182)
* activeMinutesGoalFactor = min(1, activeMinutes / activeTimeGoal) (:194-201)
* getActiveCaloriesTotal = сумма DailyTotals.activeCalories / 1000 => ккал (:67-79)
* getDistanceTotal: если steps>0 && distance>0 -> distance (см), иначе steps × stepLengthCm; результат × 0.01 = метры (:144-164)
* getActiveMinutes: StepAnalysis.calculateStepSessions(...) -> calculateSummary(...), (end−start)/1000/60 (:203-215)
* getRestingCaloriesTotal — среднее по устройствам с restingCalories > 0 (:81-102)

### 2.4 Sleep score

Устройство: последний SleepScoreSample (Garmin = GARMIN_SLEEP_STATS_SAMPLE.sleepScore) до выбранного момента, max по устройствам (PulseSleepActivity.java:77-85; DashboardFragment.java:1472-1479).

Fallback, если устройство скора не дало (PulseSleepActivity.java:150-158; дубль DashboardFragment.java:1524-1531):

```
asleep = deepMin + lightMin + remMin
total  = asleep + awakeMin
s  = 70.0 * asleep / goalMin                 // доля длительности
s += 30.0 * (deepMin + remMin) / total       // доля качества, если total > 0
score = clamp(round(s), 1, 100)
```

Пороги/цвета (PulseSleepActivity.java:213-221; DashboardFragment.java:1565-1570): >=85 «Excellent» pulse_mint; >=70 «Good» pulse_neon_cyan; >=50 «Fair» pulse_ring_cal; иначе «Poor» pulse_ring_hr.

### 2.5 Сессии сна (activities/charts/SleepAnalysis.java)

* MIN_SESSION_LENGTH = 5*60 сек, MAX_WAKE_PHASE_LENGTH = 60*60 сек (:27-28).
* Сессия рвётся при шагах (steps>0 && != NOT_MEASURED) или паузе без сна > 60 мин (:52-66).
* Длительности стадий = сумма разниц таймстемпов соседних семплов по ActivityKind (:68-92).
* Окно поиска ночи в Pulse: до 12:00 выбранного дня минус 24 часа (toSec = полдень, fromSec = toSec − 24*3600; PulseSleepActivity.java:88-93, DashboardFragment.java:1481-1487). Берётся сессия с максимальным light+deep+rem.
* Nap = другая сессия того же окна с asleep >= 10 минут (PulseSleepActivity.java:126-136).

### 2.6 Intensity minutes

week = сумма(moderate + 2×vigorous) с понедельника; today — то же с 00:00 текущего дня (DashboardFragment.java:757-780). Фактор карточки = today / pulse_intensity_goal (:978-979).

### 2.7 Streak (DashboardFragment.updateStreak, :1775-1802)

```
goal   = pref pulse_streak_goal (default "steps"; допустимо "any")
today  = YYYY-MM-DD (Locale.US), yesterday = today-1
reached = (goal=="any") ? любой из RING_METRICS имеет фактор >= 1 : goalFactor(goal) >= 1
if (просматривается сегодняшний день && reached && last != today):
    count = (last == yesterday) ? count+1 : 1
    last  = today
    best  = max(best, count)
    -> уведомление «серия N дней»
отображаемое = (last в {today, yesterday}) ? count : 0
```

Ключи: pulse_streak_goal, pulse_streak_last, pulse_streak_count, pulse_streak_best.
RING_METRICS = {steps, activetime, sleep, calories, distance} (DashboardFragment.java:1755).
Календарь серий: (а) count последовательных дней назад от pulse_streak_last (PulseStreakActivity.java:420-440); (б) фактический пересчёт по дням месяца в фоне (metGoal, :456-540) — день зачтён, если ХОТЯ БЫ ОДНО устройство достигло цели за сутки.

### 2.8 Уведомление о достигнутой цели

checkGoalReached() (DashboardFragment.java:1861-1881): ключ pulse_goal_notified_<metric> хранит дату последнего уведомления; при goalFactor >= 1 и другой дате — конфетти (PulseConfettiView) + системное уведомление, один раз в день на метрику.

### 2.9 Достижения (PulseStreakActivity.renderAchievements, :169-205)

| Бейдж | Условие |
|---|---|
| pulse_ach_first_week | daysTracked >= 7 |
| pulse_ach_first_month | daysTracked >= 30 |
| pulse_ach_week_streak | bestStreak >= 7 |
| pulse_ach_month_streak | bestStreak >= 30 |
| pulse_ach_100day_streak | bestStreak >= 100 |
| pulse_ach_10k / 20k / 30k | bestDaySteps >= 10000 / 20000 / 30000 |
| pulse_ach_100k / 500k | lifetimeSteps >= 100000 / 500000 |
| pulse_ach_million / 5m | lifetimeSteps >= 1000000 / 5000000 |
| pulse_ach_100mi | lifetimeMeters >= 160934 |
| pulse_ach_500mi | lifetimeMeters >= 804672 |

bestStreak = max(pulse_streak_best, pulse_streak_count); daysTracked — число дней с steps > 0; lifetime считается днём-за-днём от самого раннего семпла (PulseStreakActivity.java:94-167).

### 2.10 Weekly challenge (PulseWeekActivity.java:174-185)

```
floor    = stepsGoal * 7
adaptive = round(lastWeekSteps * 1.1)
target   = max(floor, adaptive); если <= 0 -> 70000
done     = weekSteps >= target
delta%   = round((weekSteps - lastWeekSteps) * 100 / lastWeekSteps)   // блок скрыт при lastWeekSteps == 0
```

Неделя = с понедельника 00:00 до «сейчас» (:82-92); глубина истории MAX_LOOKBACK_DAYS = 372 (:55, 110-115).
PR (all-time, метка NEW если поставлен на этой неделе): prSteps, prDistCm, prCal (по дням), prWorkoutSec (самая длинная запись BASE_ACTIVITY_SUMMARY), prStreak = max(pulse_streak_best, pulse_streak_count).
Средний сон недели = weekSleep / sleepDays / 60 часов; средние шаги = weekSteps / daysElapsed.

### 2.11 Health-дельта (DashboardFragment.healthDelta, :1333-1356)

```
today = hist[6]; avg = среднее по hist[0..5] только по значениям > 0
если n==0 или today==0 -> подписи нет
diff = today - avg
|diff| <= max(1, avg/20) -> «без изменений к среднему за 7 дней»
иначе стрелка вверх/вниз и |diff|
```

---

## 3. Поэкранная спецификация

Нижняя навигация — 4 вкладки, ViewPager2 (ControlCenterv2.java:234-251, 612-617): 0 today -> 1 fitness -> 2 sleep -> 3 health; все — один DashboardFragment.newInstance(section). Видимость нав-бара — display_bottom_navigation_bar.

### 3.1 Общее для всех вкладок

* Дата в шапке, навигация по дням; followingToday авто-перещёлкивает на реальное «сегодня» после полуночи (DashboardFragment.java:557-560).
* Прогрев данных в фоне (warmData(), :601-620): DailyTotals + goal-факторы -> warmExtraMetrics() -> по вкладке warmHealthHistory() / warmSleepDetail() / warmTodayExtra().
* Учитывается ТОЛЬКО первое (основное) устройство (reloadPreferences, :788-804).
* ALL_METRICS (:158-161): steps, distance, activetime, calories, intensity, sleep, heartrate, bodybattery, stress, spo2, hrv, respiration.
* «Последние значения» (warmExtraMetrics, :722-786) = getLatestSample(конец дня) с проверкой timestamp >= начало дня: bodybattery, stress, spo2, heartrate (resting, > 0), hrv, respiration (округляется).

Карточка метрики (resolveMetric, :932-1067):

| metric | значение | factor | цвет |
|---|---|---|---|
| steps | steps | stepsGoalFactor | pulse_ring_steps |
| distance | форматированная дистанция | distanceGoalFactor | accent |
| activetime | Xh Ym / Ym | activeMinutesGoalFactor | pulse_mint |
| calories | ккал | activeCaloriesGoalFactor | pulse_ring_hr |
| intensity | today-минуты (в подписи неделя) | today / pulse_intensity_goal | pulse_neon_cyan |
| sleep | Xh Ym | sleepMinutesGoalFactor | pulse_purple |
| heartrate | bpm | (v − 40) / 160 (диапазон 40–200) | pulse_ring_hr |
| bodybattery | 0..100 | v / 100 | pulse_mint |
| stress | 0..100 | v / 100 | pulse_ring_cal |
| spo2 | v% | v / 100 | pulse_neon_cyan |
| hrv | v ms | v / 120 | accent |
| respiration | вдохов/мин | (v − 6) / 24 (диапазон 6–30) | pulse_purple |

Пустое значение — строка stats_empty_value. Переходы в чарты: steps/distance -> stepsweek, activetime -> activity, calories -> calories (mode ACTIVE_CALORIES_GOAL), intensity -> pai (Garmin Intensity Minutes), sleep -> PulseSleepActivity, heartrate -> heartrate, bodybattery -> bodyenergy, stress -> stress, spo2 -> spo2, hrv -> hrvstatus, respiration -> respiratoryrate.

### 3.2 Today

* Состав метрик настраиваемый: pulse_today_metrics (default — все ALL_METRICS), редактор PulseDashboardEditActivity (drag-порядок + переключатели, флаг pulse_today_expanded).
* Герой: карусель (стр. 0 — кольцо + до двух плиток, далее плитки) либо при pulse_today_expanded=true стек «кольцо сверху + плитки во всю ширину» (:2196-2367). Индикаторы — широкая accent-«пилюля» для активной страницы (:2369-2387).
* Кольцо: метрика pulse_ring_metric (default steps; long-press -> выбор из RING_METRICS); подпись «of goal»; значение 36sp с авто-уменьшением до 18sp при ширине > 92dp (setRingValue, :1936-1947).
* Отрисовка кольца (views/PulseRingView.java): толщина 26dp×density, трек pulse_card_alt, дуга — линейный градиент accent -> pulse_ring_steps, старт −90 градусов; при progress > 1 рисуется второй круг darken(accent, 0.55); нуб-точка darken(accent, 0.6), при перевыполнении lighten(accent, 0.55); анимация 820 мс, OvershootInterpolator(1.4).
* Плашка стрика (pulse_btn_log) — число дней; тап -> диалог выбора цели / PulseStreakActivity.
* Ниже пилюль (renderTodayExtra, :2435-2513):
  * This week: weekSteps + «дистанция · N активных дней»; тап -> PulseWeekActivity. Данные — обход дней с понедельника по основному устройству, дистанция при отсутствии = steps × stepLength (:623-698).
  * Recent workouts: 3 последних BASE_ACTIVITY_SUMMARY (order by startTime desc limit 3): иконка ActivityKind + имя/лейбл + «дата · Xh Ym»; «See all» -> PulseWorkoutsActivity.
* Greeting: <12 morning, <18 afternoon, иначе evening + первое слово имени, если оно не gadgetbridge-user (:1804-1815).
* Share-карточка: заголовок — шаги, ниже 4 метрики из pulse_share_metrics (default distance,calories,activetime,sleep) + дата и pulse_streak_count (:459-549).

### 3.3 Fitness

Кураторский набор карточек {steps, distance, activetime, calories, heartrate} (:170-172) + герой-кольцо.

### 3.4 Sleep

* Карточки {respiration, hrv, bodybattery} (:173-174) + герой-кольцо (sleep -> «Xh Ym», «of goal Xh»).
* Инлайн-деталь (warmSleepDetail :1463-1531 / renderSleepDetail :1533-1634): крупный sleep score (40sp) + слово качества; «asleep · bed -> wake» (h:mm a); stage bar высотой 10dp с весами по минутам (deep pulse_purple, light pulse_neon_cyan, rem pulse_neon, awake pulse_ring_hr) + легенда с точками 8dp; 7-дневный мини-график сна (DailyTotals.getSleep() по дням, oldest..today); тап -> PulseSleepActivity.
* PulseSleepActivity:
  * score с count-up анимацией 900 мс (задержка 150 мс) + слово качества;
  * длительность и bed -> wake, плюс «N naps Xh Ym»;
  * гипнограмма: соседние семплы одной стадии сливаются, длительность сегмента = разница таймстемпов, клампится в [60, 3600] сек, вес сегмента в строке = длительность (:164-190);
  * stage bar + легенда;
  * 7 ночей: пилюли шириной 32dp, высота max(32dp, 96dp × v / max), пустая ночь = «точка» alpha 0.25, подпись EEEEE, средняя длительность по ночам с v>0 (:303-347);
  * инсайт: asleep >= goal -> «цель достигнута»; asleep < goal -> «не хватает Xh Ym до Xh Ym»; иначе — доля глубокого сна round(100 × deep / total)%.

### 3.5 Health

* Без кольца, только сетка; состав/порядок — pulse_health_metrics (default heartrate,bodybattery,stress,spo2,hrv,respiration).
* Первая метрика может быть headline (pulse_health_headline, default bodybattery): крупная карточка — чип-иконка 30dp, значение 34sp цветом метрики, дельта к 7-дневному среднему, спарклайн SparkView (заливка + точка на последнем значении, высота 58dp) (:1193-1264).
* Остальные — компактные строки: чип + название + дельта + мини-спарк 48×22dp + значение + шеврон (:1266-1331).
* История 7 дней (warmHealthHistory :1079-1154): день = СРЕДНЕЕ по семплам суток — bodybattery (energy), stress/spo2 (только > 0), hrv (value), respiration (округлённое), heartrate (resting > 0); steps/distance/sleep/activetime — из DailyTotals, calories = activeCalories/1000.
* Строка «Edit» -> PulseDashboardEditActivity с pulse_edit_section = "health".

### 3.6 Week in Review (PulseWeekActivity)

Диапазон недели (MMM d – сегодня), челлендж (статус/цель/прогресс-бар), дельта к прошлой неделе (up/down/same с цветами pulse_mint / pulse_text_dim), ячейки: дистанция, ккал, средний сон, activeDays / 7, лучший день, средние шаги; список PR с меткой NEW. Анимации: поочерёдное появление, count-up шагов, анимация бара челленджа.
Пуш «week in review» — воскресенье 19:00 (RECAP_HOUR=19), самоперепланирование, notification id 0x50B1, PendingIntent request codes 0x50B1/0x50B2 (util/PulseWeeklyRecapReceiver.java:36-70).

### 3.7 Achievements / Streak (PulseStreakActivity)

* Текущая серия (правило §2.7), выбор цели (диалог: any + steps, activetime, sleep, calories, distance).
* Календарь месяца: сетка Sunday-first, ячейка 46dp / кружок 38dp; «сегодня» — pulse_streak_today, зачтённый день — pulse_streak_hit (текст pulse_bg), будущие дни — pulse_card_alt; перелистывание месяцев.
* Lifetime-плитки: шаги, дистанция, ккал, часы сна, число отслеженных дней.
* Сетка бейджей (§2.9): заблокированные alpha 0.45 + серый тинт; тап по разблокированному — шаринг PNG 1080px (pulse_badge_share_card).

### 3.8 Workouts (PulseWorkoutsActivity)

Все BASE_ACTIVITY_SUMMARY (order by startTime desc): иконка вида активности (accent-тинт) + имя или лейбл вида + «дата · Xh Ym»; тап -> WorkoutDetailsActivity (передаются itemsFilter=[id], position=0, EXTRA_DEVICE); пусто -> плашка workouts_empty.

### 3.9 Settings (res/xml/preferences.xml — Pulse-урезанные)

* General: general_autostartonboot (true), permission_pestering (true), show_changelog (true), general_autoconnectonbluetooth (false), general_reconnectonlytoconnected (true), general_prefs_key_sort_by_last_connected_ts (false), language (default); ЕДИНИЦЫ: unit_distance (default imperial), unit_temperature (fahrenheit), unit_weight (pound); datetime_synconconnect (true); локация (location_latitude/longitude, use_updated_location_if_available).
* About you -> AboutUserPreferencesActivity; Goals -> PulseGoalsActivity (res/xml/pulse_goals.xml, §2.2; сон — пикер часы (0..14) + минуты (0..59), сохраняется строкой в минутах; при изменении любой цели — onSendConfiguration(key) + ACTION_REFRESH_DEVICELIST).
* Appearance: pref_key_theme (дефолт приложения — ТЁМНАЯ тема, GBApplication.java:674-677), pref_key_theme_amoled_black (false), pulse_accent (default blue; значения blue, violet, coral, mint, pink — arrays.xml:5391-5402; применяется как theme-overlay PulseAccent* в AbstractGBActivity.init, :120-133; цвет читается через attr pulseAccent — GBApplication.getAccentColor, :693-696), block_screenshots (false), pref_refresh_on_swipe (true), display_add_device_fab (true), display_bottom_navigation_bar (true).
* Dashboard (res/xml/dashboard_preferences.xml): pref_dashboard_edit_layout, pulse_ring_metric (default steps).
* Weather (PulseWeatherActivity): pulse_weather_source (auto/off), pulse_weather_city -> геокодинг в pulse_weather_lat/pulse_weather_lon, pulse_weather_auto (true), pulse_weather_refresh.
* Advanced: pulse_send_logs -> PulseReportActivity (crash trace в extra pulse_crash_trace).
* Онбординг: pulse_onboarded, выбор accent и weather-source (PulseOnboardingActivity.java:41, 157-164).

### 3.10 Домашний виджет (Widget.java)

3 настраиваемых слота, ключ pulse_widget_metrics_<appWidgetId>, default steps,distance,sleep (:165-168).
Слоты: steps (max = stepsGoal), distance (см -> м, fallback steps × stepLength, max = distanceGoal), calories (activeCalories/1000, max = caloriesGoal), sleep (минуты, max = sleepGoal), heartrate (последний resting HR, без бара), bodybattery (последний, max = 100).
В шапке — имя/алиас устройства и заряд getBatteryLevel(0) (если connected и > 1); клики: обновление данных (onFetchRecordedData), ControlCenter, чарты.

---

## 4. Палитра и визуальные константы

res/values/colors.xml:135-149 (светлая; тёмные — values-night/colors.xml):
pulse_bg #F4F5F7, pulse_card #FFFFFF, pulse_card_alt #E4E7EC, pulse_neon #1488D6, pulse_neon_cyan #0E9DC4, pulse_ring_steps #0E9DC4, pulse_ring_hr #E5484D, pulse_ring_cal #E07B1F, pulse_purple #6A40E0, pulse_mint #1FA877, pulse_text #16181D, pulse_text_dim #6B6F78; акценты: accent_blue #1488D6, accent_violet #6A40E0, далее coral/mint/pink.

Размеры: карточка метрики — радиус 20dp, паддинг 14dp, значение 24sp (шрифт unbounded), бар 5dp; кольцо 158×158dp; health-headline радиус 22dp, паддинг 18dp; спарклайн 58dp (headline) / 48×22dp (строка); dashboard-виджеты — радиус 22dp, паддинг 12dp, elevation 0.

---

## 5. Что важно повторить в Go/QML

1. Два масштаба времени: секунды для activity-семплов, миллисекунды для time-семплов.
2. Ночь считается по окну «полдень − 24 часа»; сон в DailyTotals — по окну со сдвигом −12 часов.
3. Все goal-факторы клампятся единицей, но кольцо умеет второй круг при progress > 1 — там фактор берётся напрямую (steps/goal, sleepMin/goal; DashboardFragment.java:1962-1996).
4. Дистанция без GPS доопределяется как steps × stepLengthCm, где stepLength = height × 0.43.
5. Streak/achievements/goal-notified/накопительные настройки — состояние в prefs, не в БД (для Go-демона это отдельный key-value store или QSettings фронтенда).
6. Активные калории в БД хранятся ×1000 — делить на 1000 при показе (DashboardUtils.java:78, Widget.java:196).
7. Дашборд агрегирует ТОЛЬКО первое устройство, а стрик-календарь — «любое устройство»; при одном Garmin-устройстве разницы нет, но логику стоит сохранить явно.