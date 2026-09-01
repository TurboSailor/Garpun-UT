# Garmin FIT: полная спецификация парсинга/генерации и импорта в БД (Pulse / Gadgetbridge)

Пути — относительно `pulse-main/app/src/main/java/nodomain/freeyourgadget/gadgetbridge/`, если не указано иное. Кодогенератор и профиль: `pulse-main/FitCodeGenerator/`.

## 0. Архитектура FIT-слоя

- Заголовок/CRC/цикл разбора — `service/devices/garmin/fit/FitFile.java`
- Заголовок записи — `.../fit/RecordHeader.java`
- Определение записи — `.../fit/RecordDefinition.java`
- Данные записи — `.../fit/RecordData.java`
- Определения полей — `.../fit/FieldDefinition.java`, `DevFieldDefinition.java`, `FieldDefinitionFactory.java`
- Базовые типы — `.../fit/baseTypes/BaseType*.java`
- Таблица сообщений — ГЕНЕРИРУЕТСЯ: `NativeFITMessages.java`, `messages/Fit*.java`, `enums/*.java` из `FitCodeGenerator/src/main/resources/fit_profile.json` генератором `FitCodeGen.java`
- Импорт в БД — `.../fit/FitImporter.java`, `FitAsyncProcessor.java`
- Тренировки — `devices/garmin/GarminWorkoutParser.java`
- CRC — `service/devices/garmin/ChecksumCalculator.java`
- Чтение байтов — `service/devices/garmin/GarminByteBufferReader.java`

ВАЖНО: `messages/Fit*.java`, `NativeFITMessages.java`, `enums/*` и большинство `FieldDefinition<Enum>` в репозитории ОТСУТСТВУЮТ — генерируются при сборке (FitCodeGen.main, FitCodeGen.java:176-181, выход `app/build/generated/sources/fit/...`). Вручную написаны только FitMonitoring, FitRecord, FitSession, FitStressLevel, FitWeather (наследуют сгенерированные AbstractFit*). Единственный источник истины по номерам сообщений/полей — fit_profile.json (4515 строк: messages[], enumerations[], devices[]).

## 1. Побайтовый формат FIT

### 1.1 Заголовок файла (FitFile.Header, FitFile.java:236-330)

    offset size поле
    0      1    header_size        12 или 14; <12 -> FitParseException "Too short header"
    1      1    protocol_version   исходящие: 16
    2      2    profile_version    LE; исходящие: 21117
    4      4    data_size          LE uint32, без header и без CRC
    8      4    magic = 0x5449462E (".FIT", читается как LE int)
    12     2    header_crc         только если header_size == 14

hasCRC = (headerSize == 14); при генерации headerSize = hasCRC ? 14 : 12. Header CRC: если прочитанное значение != 0 — сверяется с computeCrc(buf, 0, headerSize-2); 0 = «не задан» (FitFile.java:296-302). Заголовок всегда LITTLE_ENDIAN (FitFile.java:82-83).

### 1.2 Цикл разбора (FitFile.parseIncoming, FitFile.java:78-138)

    pos = header_size
    while pos < header_size + data_size:
        hdr = RecordHeader(readByte())
        if hdr.timeOffset != nil:                       // compressed timestamp
            if ref == nil: error "Got compressed timestamp without knowing current timestamp"
            if timeOffset >= (ref & 0x1F): ref = (ref &~ 0x1F) + timeOffset
            else:                          ref = (ref &~ 0x1F) + timeOffset + 0x20
        if hdr.isDefinition:
            def = RecordDefinition.parseIncoming(...)
            if hdr.developerData:
                for rd in разобранные: if rd.msg == FIELD_DESCRIPTION(206): def.populateDevFields(rd)
            defsByLocalType[hdr.localMessageType] = def   // последнее определение побеждает
        else:
            def = defsByLocalType[hdr.localMessageType]
            if def != nil:
                data = FitRecordDataFactory.create(def, hdr)
                newTs = data.parseDataMessage(reader, ref)
                if newTs != nil: ref = newTs
    fileCrc   = readShort()                      // LE uint16
    actualCrc = computeCrc(fileBytes, 0, pos-2)  // ВКЛЮЧАЯ header
    if fileCrc != actualCrc: FitParseException
    if pos < limit: WARN "There are N bytes after the fit file"  // склеенные FIT-файлы НЕ поддерживаются

### 1.3 Record header — 1 байт (RecordHeader.java:29-38)

    bit7 = 1 -> compressed timestamp:
        definition=false, developerData=false
        localMessageType = (b >> 5) & 0x03      // только 0..3
        timeOffset       =  b & 0x1F            // 5 бит, секунды
    bit7 = 0 -> normal:
        definition       = (b & 0x40) != 0
        developerData    = (b & 0x20) != 0
        localMessageType =  b & 0x0F            // 0..15

Генерация (RecordHeader.java:63-83): definition -> lmt | 0x40 | (dev?0x20:0); data -> lmt | (dev?0x20:0); compressed -> timeOffset | (lmt << 5).

### 1.4 Definition record (RecordDefinition.java:29-60)

    1 байт  reserved             читается и игнорируется; при генерации 0
    1 байт  architecture         0x01 -> BIG_ENDIAN, иначе LITTLE_ENDIAN
    2 байта global_message_num   в выбранном порядке байт
    1 байт  num_fields
    num_fields x 3: field_def_number, size (в БАЙТАХ, кратно базовому -> массив), base_type
    if header.developerData:
        1 байт num_dev_fields
        num_dev_fields x 3: field_number, size, developer_data_index

При генерации (RecordDefinition.java:90-104) dev-поля НЕ пишутся.

### 1.5 Data record

Тело = значения полей в порядке объявления, общий размер = сумма size. Dev-поля идут ПОСЛЕ обычных (RecordData конструктор :45-85). Поле 253 при чтении возвращает текущий timestamp и обновляет ref (RecordData.FieldData.parseDataMessage :375-381).

### 1.6 CRC (ChecksumCalculator.java:22-49)

    T = 0x0000,0xCC01,0xD801,0x1400,0xF001,0x3C00,0x2800,0xE401,
        0xA001,0x6C00,0x7800,0xB401,0x5000,0x9C01,0x8801,0x4400
    crc = ((crc>>4)&0x0FFF) ^ T[crc&0xF] ^ T[b&0xF]
    crc = ((crc>>4)&0x0FFF) ^ T[crc&0xF] ^ T[(b>>4)&0xF]

При чтении CRC файла считается от смещения 0; при генерации — от headerSize (FitFile.java:129-131 vs :174-176). Повторять как в исходнике.

## 2. Базовые типы (baseTypes/BaseType.java:9-27)

| BaseType | id | размер | знак | invalid | результат decode |
|---|---|---|---|---|---|
| ENUM | 0x00 | 1 | unsigned | 0xFF | int (float если scale!=1) |
| SINT8 | 0x01 | 1 | signed | 0x7F | int |
| UINT8 | 0x02 | 1 | unsigned | 0xFF | int |
| SINT16 | 0x83 | 2 | signed | 0x7FFF | int |
| UINT16 | 0x84 | 2 | unsigned | 0xFFFF | int |
| SINT32 | 0x85 | 4 | signed | 0x7FFFFFFF | long (double если scale!=1) |
| UINT32 | 0x86 | 4 | unsigned | 0xFFFFFFFF | long |
| STRING | 0x07 | 1 | unsigned | 0x00 | UTF-8 до первого нуля |
| FLOAT32 | 0x88 | 4 | - | NaN | float |
| FLOAT64 | 0x89 | 8 | - | NaN | double |
| UINT8Z | 0x0A | 1 | unsigned | 0x00 | int |
| UINT16Z | 0x8B | 2 | unsigned | 0 | int |
| UINT32Z | 0x8C | 4 | unsigned | 0 | long |
| BYTE | 0x0D | 1 | unsigned | 0xFF | int |
| SINT64 | 0x8E | 8 | signed | 0x7FFFFFFFFFFFFFFF | long |
| UINT64 | 0x8F | 8 | unsigned | 0xFFFFFFFFFFFFFFFF | long |
| UINT64Z | 0x90 | 8 | unsigned | 0 | long |

Неизвестный id -> fallback на BYTE с WARN (BaseType.fromIdentifier :43-51). Типизация (BaseType.decode :66-99): 1-2-байтовые + BYTE/ENUM -> int (float при scale!=1); 4/8-байтовые -> long (double при scale!=1).

### 2.1 Формулы (BaseTypeByte/Short/Int/Long)

    decode: raw = read(signed|unsigned)
            if raw < min || raw > max -> nil
            if raw == invalid         -> nil
            value = (raw / scale) - offset
    encode: nil -> записать invalid
            raw = (long)((value + offset) * scale)   // value как double, дроби не режутся
            if raw вне [min,max] -> invalid

### 2.2 Массивы и строки (RecordData.FieldData :375-425)

- size > baseSize -> массив из size/baseSize элементов.
- STRING: читается size байт, обрезается по первому 0x00, UTF-8. Encode: UTF-8 обрезается до size-1 + 0x00.
- Инвалидизация: STRING -> нули; иначе size/baseSize раз invalid.

## 3. Спец-типы полей (fieldDefinitions/, фабрика FieldDefinitionFactory.java:33-50)

| type в json | класс | семантика |
|---|---|---|
| TIMESTAMP | FieldDefinitionTimestamp | scale=1, offset=-GARMIN_TIME_EPOCH -> decode даёт Unix-секунды |
| COORDINATE | FieldDefinitionCoordinate | deg = semicircles * 180/2^31 (GarminUtils.java:21); обратно round(deg / SEMICIRCLE_DEGREES) |
| ALARM | FieldDefinitionAlarm | uint16 минут от полуночи <-> LocalTime(v/60, v%60) |
| HR_TIME_IN_ZONE | FieldDefinitionHrTimeInZone | принудительно scale=1000, offset=0 (мс->с) |
| HR_ZONE_HIGH_BOUNDARY | FieldDefinitionHrZoneHighBoundary | массив Integer, уд/мин |
| TEMPERATURE | FieldDefinitionTemperature | прозрачный; K->°C делается в FitWeather.Builder |
| FILE_TYPE | FieldDefinitionFileType | FILETYPE.fromDataTypeSubType(128, raw); encode пишет subType |
| DAY_OF_WEEK | FieldDefinitionDayOfWeek | java.time.DayOfWeek |
| BOOLEAN | FieldDefinitionBoolean | 0/1 |
| ARRAY | FieldDefinitionArray | наследник; массив разбирается в FieldData |
| SleepStage, HrvStatus, WeatherCondition, WeatherAqi, WeatherReport, BatteryStatus, ExerciseCategory | сгенерированные | decode: num->enum, encode: enum.num |

Сопоставление поля с профилем — NativeFITMessage.getFieldDefinition(id, size, baseType) (NativeFITMessage.java:117-170): профиль ENUM + файл UINT8 -> DEBUG, принимается; UINT32Z vs UINT32 -> INFO; иное расхождение -> WARN; size==1 при объявленных UINT16/32/64 -> base type переопределяется на UINT8 (совместимость с COROS); имя/scale/offset/тип из профиля, размер — из файла. Неизвестное сообщение -> поля с пустым именем. Поле 253/size4/UINT32 всегда получает FieldDefinitionTimestamp("253_timestamp") (FieldDefinition.java:60-79).

### 3.1 Developer fields

FIELD_DESCRIPTION (206): developer_data_index(0,UINT8), field_definition_number(1,UINT8), fit_base_type_id(2,UINT8), field_name(3,STRING[64]), array(4), components(5), scale(6,UINT8), offset(7,SINT8), units(8,STRING[16]), bits(9), accumulate(10), fit_base_unit_id(13,UINT16), native_mesg_num(14,UINT16), native_field_num(15,UINT8) — fit_profile.json:2109-2128. RecordDefinition.populateDevFields (:111-135) матчит по (field_definition_number, developer_data_index). Без описания dev-поле читается как BYTE по объявленному размеру (RecordData.java:60-77). getFieldByNumber(n) (:160-192): dev-поле с совпадающими native_mesg_num+native_field_num имеет ПРИОРИТЕТ над нативным.

## 4. Время и масштабы

GARMIN_TIME_EPOCH = 631065600 (1989-12-31T00:00:00Z), GarminTimeUtils.java:9. unix = garmin + 631065600; garmin = unix - 631065600; millis = (garmin+EPOCH)*1000; unixTimeToGarminDayOfWeek(unix) = DayOfWeek.getValue() % 7 (вс = 0). Все поля с "type":"TIMESTAMP" декодируются сразу в Unix-секунды (offset = -EPOCH). RecordData.computedTimestamp — timestamp записи: поле 253, либо compressed-timestamp, либо унаследованный (RecordData.java:36-41, 105-117).

timestamp16 (MONITORING поле 26), FitMonitoring.computeTimestamp (messages/FitMonitoring.java:30-51):

    refGarmin = unixToGarmin(lastMonitoringUnixTs)
    diff = (timestamp16 & 0xFFFF) - (refGarmin & 0xFFFF)
    if diff < -32768: diff += 65536
    if diff >  32768: diff -= 65536
    ts = lastMonitoringUnixTs + diff

Нет timestamp16 -> lastMonitoringTimestamp, иначе computedTimestamp. scale/offset берутся из fit_profile.json; в Java хранятся как int (дробные scale округляются генератором, FitCodeGen.java:527-543).

## 5. Используемые global message numbers

### 5.1 Что реально разбирается

FitImporter.importFile (fit/FitImporter.java:178-500): FILE_ID(0), STRESS_LEVEL(227), SLEEP_DATA_INFO(273), SLEEP_DATA_RAW(274), SLEEP_RESTLESS_MOMENTS(382), SLEEP_STATS(346), SLEEP_STAGE(275), NAP(412), MONITORING(55), SPO2(269), RESPIRATION_RATE(297), EVENT(21), RECORD(20), SESSION(18), PHYSIOLOGICAL_METRICS(140), SPORT(12), TIME_IN_ZONE(216), USER_PROFILE(3), HRV_SUMMARY(370), HRV_VALUE(371), MONITORING_INFO(103), TRAINING_LOAD(378), MONITORING_HR_DATA(211), DEVICE_STATUS(104), HILL_SCORE(402), TRAINING_READINESS(369), ENDURANCE_SCORE(403), FUNCTIONAL_METRICS(356), MAX_MET_DATA(229).
GarminWorkoutParser.handleRecord (devices/garmin/GarminWorkoutParser.java:228+): LAP(19), SET(225), DEVICE_INFO(23), FILE_CREATOR(49), ACTIVITY(34), WORKOUT(26), USER_METRICS(79), DIVE_SETTINGS(258), DIVE_GAS(259), DIVE_SUMMARY(268), TANK_SUMMARY(323).
Исходящие/служебные: WEATHER(128), ALARM_SETTINGS(222), DEVICE_SETTINGS(2), CAPABILITIES(1), FIELD_DESCRIPTION(206), COURSE(31)/COURSE_POINT(32)/RECORD/EVENT/LAP (GpxRouteFileConverter).

### 5.2 MONITORING — 55 (fit_profile.json:1141-1180)

| num | имя | base | scale/offset | ед. |
|---|---|---|---|---|
|0|device_index|UINT8|-|-|
|1|calories|UINT16|-|kcal|
|2|distance|UINT32|-|-|
|3|cycles|UINT32|-|шаги|
|4|active_time|UINT32|scale 1000|с|
|5|activity_type|ENUM|-|-|
|6|activity_subtype|ENUM|-|-|
|7|activity_level|ENUM|-|-|
|8|distance_16|UINT16|-|100*м|
|9|cycles_16|UINT16|-|2*шага|
|10|active_time_16|UINT16|-|с|
|11|local_timestamp|UINT32|-|с|
|12/14/15|temperature/_min/_max|SINT16|scale 100|°C|
|16|activity_time|UINT16[8]|-|мин|
|19|active_calories|UINT16|-|kcal|
|24|current_activity_type_intensity|BYTE|-|&0x1F = тип, >>5&0x7 = интенсивность|
|25|timestamp_min_8|UINT8|-|мин|
|26|timestamp_16|UINT16|-|с (см. раздел 4)|
|27|heart_rate|UINT8|-|bpm|
|28|intensity|UINT8|scale 10|-|
|29|duration_min|UINT16|-|мин|
|30|duration|UINT32|-|с|
|31/32|ascent/descent|UINT32|scale 1000|м|
|33|moderate_activity_minutes|UINT16|-|мин|
|34|vigorous_activity_minutes|UINT16|-|мин|
|35/36|total_ascent/total_descent|UINT32|scale 1000|м|
|37/38|moderate_activity/vigorous_activity|UINT16|-|мин|
|251|pad|BYTE[]|-|-|
|253|timestamp|UINT32 TIMESTAMP|-|Unix|
|254|message_index|UINT16|-|-|

Импортёр берёт 27, 3, 2, 19, 33, 34, 5/24, 26/253 (FitImporter.java:820-880).

### 5.3 MONITORING_INFO — 103
local_timestamp(0,UINT32,с), activity_type(1,ENUM[]), steps_to_distance(3,UINT16[],scale 5000,м/цикл), steps_to_calories(4,UINT16[],scale 5000), resting_metabolic_rate(5,UINT16,kcal/сут), cycles_goal(7,UINT32,scale 2), timestamp(253), message_index(254). Импорт берёт только 5 (FitImporter.java:389-399).

### 5.4 MONITORING_HR_DATA — 211
resting_heart_rate(0,UINT8,bpm), current_day_resting_heart_rate(1,UINT8,bpm), timestamp(253). Приоритет — поле 1 (FitImporter.java:415-433).

### 5.5 STRESS_LEVEL — 227
stress_level_value(0,SINT16), stress_level_time(1,UINT32,TIMESTAMP), average_stress(2,SINT8), body_energy(3,SINT8). Импорт: stress >= 0 -> GarminStressSample; body_energy -> GarminBodyEnergySample (FitImporter.java:208-231).

### 5.6 SPO2 — 269
reading_spo2(0,UINT8,%), reading_confidence(1,UINT8), mode(2,ENUM), timestamp(253). Импорт: spo2 > 0; mode 1 -> MANUAL, 3 -> AUTOMATIC, иначе UNKNOWN (FitImporter.java:288-313).

### 5.7 RESPIRATION_RATE — 297
respiration_rate(0,SINT16,scale 100,вдох/мин), timestamp(253). Импорт: > 0.

### 5.8 Сон
- SLEEP_STAGE(275): sleep_stage(0,ENUM->SleepStage), timestamp(253). SleepStage: 0 UNMEASURABLE, 1 AWAKE, 2 LIGHT, 3 DEEP, 4 REM (fit_profile.json:3712-3720).
- SLEEP_STATS(346): combined_awake_score(0), awake_time_score(1), awakenings_count_score(2), deep_sleep_score(3), sleep_duration_score(4), light_sleep_score(5), overall_sleep_score(6), sleep_quality_score(7), sleep_recovery_score(8), rem_sleep_score(9), sleep_restlessness_score(10), awakenings_count(11), interruptions_score(14), average_stress_during_sleep(15,UINT16,scale 100). Все UINT8 кроме 15. Импорт берёт только 6.
- SLEEP_DATA_INFO(273): unk0(0,UINT8), sample_length(1,UINT16,с), local_timestamp(2,UINT32), unk3(3,ENUM), version(4,STRING), timestamp(253).
- SLEEP_DATA_RAW(274): bytes(0,BYTE) — формат неизвестен, используется только количество записей.
- SLEEP_RESTLESS_MOMENTS(382): sleep_start(0,UINT32,с), restless_moments_count(1,UINT8), durations(2,UINT8[]).
- NAP(412): start_timestamp(0,TIMESTAMP), start_tz_offset(1,SINT16,мин), end_timestamp(2,TIMESTAMP), end_tz_offset(3,SINT16), feedback(4,ENUM), deleted(6,BOOLEAN), updated_timestamp(7,TIMESTAMP), timestamp(253), message_index(254).
- Есть в профиле, но не импортируются: SLEEP_SCHEDULE(379), DAILY_SLEEP(384), SLEEP_DEMAND(410), SLEEP_SUMMARY(411), SLEEP_DISRUPTION_SEVERITY_PERIOD(470), SLEEP_DISRUPTION_OVERNIGHT_SEVERITY(471).

### 5.9 HRV
- HRV_SUMMARY(370): weekly_average(0), last_night_average(1), last_night_5_min_high(2), baseline_low_upper(3), baseline_balanced_lower(4), baseline_balanced_upper(5) — все UINT16, scale 128, мс; status(6,ENUM->HrvStatus); timestamp(253). HrvStatus: 0 NONE, 1 POOR, 2 LOW, 3 UNBALANCED, 4 BALANCED.
- HRV_VALUE(371): value(0,UINT16,scale 128,мс), timestamp(253).
- Импорт округляет до целых мс (FitImporter.java:341-385). HRV(78) и RAW_BBI(372) не импортируются.

### 5.10 Прочие метрики -> GenericMetricSample
- TRAINING_LOAD(378): training_load_acute(3,UINT16), training_load_chronic(4,UINT16), daily_acute_chronic_workload_ratio(5,UINT8,scale 10), timestamp(253).
- DEVICE_STATUS(104): battery_voltage(0,UINT16,scale 1000), battery_level(2,UINT8,%), temperature(3,SINT8,°C), timestamp(253) -> BatteryLevel(ts в СЕКУНДАХ, index 0, level).
- HILL_SCORE(402): hill_score(0,UINT8), hill_strength(1,UINT8), hill_endurance, level -> GARMIN_HILL_SCORE/_STRENGTH/_ENDURANCE (value > 0).
- TRAINING_READINESS(369): training_readiness(0,UINT8), level(1,ENUM) -> GARMIN_TRAINING_READINESS.
- ENDURANCE_SCORE(403): endurance_score(0,UINT16), level(1,ENUM) -> GARMIN_ENDURANCE_SCORE.
- FUNCTIONAL_METRICS(356): functional_threshold_power, cycling_lactace_threshold_hr, running_lactate_threshold_power, running_lactate_threshold_hr -> GARMIN_FUNCTIONAL_THRESHOLD_POWER, GARMIN_RUNNING_LACTATE_THRESHOLD_POWER.
- MAX_MET_DATA(229): update_time(0,TIMESTAMP), vo2_max(2,UINT16,scale 10,мл/кг/мин), max_met_category -> GARMIN_MET_MAX_VO2.
- MONITORING_ALTITUDE(279): altitude(0,UINT32,scale 5,offset 500,м) — типовое кодирование высоты.

### 5.11 FILE_ID — 0 (fit_profile.json:4-17)
type(0,ENUM->FILE_TYPE), manufacturer(1,UINT16), product(2,UINT16), serial_number(3,UINT32Z), time_created(4,UINT32,TIMESTAMP), number(5,UINT16), manufacturer_partner(6,UINT16), product_name(8,STRING[20]), pad(251,BYTE[]). FitFile.getFileType() берёт ПЕРВУЮ запись (FitFile.java:180-198).

### 5.12 EVENT — 21
event(0,ENUM), event_type(1,ENUM), data16(2,UINT16), data(3,UINT32), event_group(4), score(7), opponent_score(8), front/rear_gear*(9..12), device_index(13), activity_type(14), start_timestamp(15,TIMESTAMP), radar_*(21..24), timestamp(253). Для сна: event=74, event_type=0 (начало) / 1 (конец), data = garmin-epoch.

### 5.13 RECORD — 20 (ключевые; полностью fit_profile.json:737-840)
latitude(0,SINT32,COORDINATE), longitude(1), altitude(2,UINT16,scale 5,offset 500,м), heart_rate(3), cadence(4), distance(5,UINT32,scale 100,м), speed(6,UINT16,scale 1000,м/с), power(7,Вт), grade(9,SINT16,scale 100,%), temperature(13,SINT8), cycles(18), total_cycles(19), gps_accuracy(31,м), vertical_speed(32,scale 1000), calories(33), oscillation(39,scale 10,мм), stance_time_percent(40,scale 100), stance_time(41,scale 10,мс), enhanced_speed(73,UINT32,scale 1000), enhanced_altitude(78,UINT32,scale 5,offset 500), vertical_ratio(83,scale 100), stance_time_balance(84,scale 100), step_length(85,scale 10,мм), performance_condition(90,SINT8), depth(92,scale 1000,м), cns_load(97,%), n2_load(98,%), respiration_rate(99,UINT8), enhanced_respiration_rate(108,UINT16,scale 100), spo2(133,%), wrist_heart_rate(136), stamina(138,%), stamina_potential(137,%), core_temperature(139,scale 100), body_battery(143), ebike_battery_level(118,%), timestamp(253). FitRecord.toActivityPoint() (messages/FitRecord.java:29-77) даёт приоритет enhanced_* над базовыми.

### 5.14 SESSION — 18 (ключевые; полностью fit_profile.json:393-589)
event(0), event_type(1), start_time(2,TIMESTAMP), start_latitude/longitude(3,4), sport(5,ENUM), sub_sport(6,ENUM), total_elapsed_time(7,UINT32), total_timer_time(8,scale 1000,с), total_distance(9,scale 100,м), total_cycles(10), total_calories(11), avg_speed(14,scale 1000), max_speed(15), average_heart_rate(16), max_heart_rate(17), avg_cadence(18), max_cadence(19), avg_power(20), max_power(21), total_ascent(22,м), total_descent(23,м), total_training_effect(24,scale 10), num_laps(26), swim_stroke(43), pool_length(44,scale 100), avg_altitude(49,scale 5,offset 500), max_altitude(50), min_altitude(71), avg/max_temperature(57,58), total_moving_time(59,scale 1000), min_heart_rate(64), time_in_hr_zone(65,UINT32[],scale 1000,с) и 66-68, sport_profile_name(110,STRING[64]), timestamp(253). LAP(19) — аналогичный набор (fit_profile.json:590-736).

### 5.15 WEATHER — 128 (fit_profile.json:1421-1446)

| num | имя | base | тип | ед. |
|---|---|---|---|---|
|0|weather_report|ENUM|WeatherReport|0 current, 1 hourly_forecast, 2 daily_forecast|
|1|temperature|SINT8|TEMPERATURE|°C|
|2|condition|ENUM|WeatherCondition|-|
|3|wind_direction|UINT16|-|градусы|
|4|wind_speed|UINT16|-|мм/с|
|5|precipitation_probability|UINT8|-|%|
|6|temperature_feels_like|SINT8|TEMPERATURE|°C|
|7|relative_humidity|UINT8|-|%|
|8|location|STRING[15]|-|-|
|9|observed_at_time|UINT32|TIMESTAMP|-|
|10/11|observed_location_lat/long|SINT32|COORDINATE|semicircles|
|12|day_of_week|ENUM|DAY_OF_WEEK|-|
|13/14|high_temperature/low_temperature|SINT8|TEMPERATURE|°C|
|15|dew_point|SINT8|TEMPERATURE|°C|
|16|uv_index|FLOAT32|-|0..10|
|17|air_quality|ENUM|WeatherAqi|0 GOOD..5 HAZARDOUS|
|18|atmospheric_pressure|UINT32|-|Па|
|253|timestamp|UINT32|TIMESTAMP|-|

WeatherCondition: 0 CLEAR, 1 PARTLY_CLOUDY, 2 MOSTLY_CLOUDY, 3 RAIN, 4 SNOW, 5 WINDY, 6 THUNDERSTORMS, 7 WINTRY_MIX, 8 FOG, 9/10 UNK, 11 HAZY, 12 HAIL, 13 SCATTERED_SHOWERS, 14 SCATTERED_THUNDERSTORMS, 15 UNKNOWN_PRECIPITATION, 16 LIGHT_RAIN, 17 HEAVY_RAIN, 18 LIGHT_SNOW, 19 HEAVY_SNOW, 20 LIGHT_RAIN_SNOW, 21 HEAVY_RAIN_SNOW, 22 CLOUDY (fit_profile.json:3753-3780). Маппинг OWM->FIT: fieldDefinitions/FieldDefinitionWeatherCondition.java:71-155; нет соответствия -> 255.

### 5.16 Настройки/устройство
- DEVICE_SETTINGS(2): alarms_time(8,UINT16[]), alarms_unk5(9,ENUM[]), alarms_enabled(28,ENUM[]), alarms_repeat (через FitDeviceSettings.Builder), utc_offset(1), time_zone_offset(5,SINT8[]), clock_time(39,TIMESTAMP), activity_tracker_enabled(36), auto_goal(45).
- ALARM_SETTINGS(222): time(0,UINT16,ALARM), repeat(1,UINT32Z), далее enabled, sound, backlight, time_created, snooze, label, message_index(254) (fit_profile.json:2218-2234).
- DEVICE_INFO(23): device_index(0), device_type(1), manufacturer(2), serial_number(3,UINT32Z), product(4), software_version(5), hardware_version(6), cum_operating_time(7,с), battery_voltage(10,UINT16,scale 256,В), battery_status(11->BatteryStatus), descriptor(19,STRING), product_name(27,STRING), ble_address(29,UINT8[6]), battery_level(32,%), timestamp(253), message_index(254).
- CAPABILITIES(1): languages(0,UINT8Z[]), sports(1,UINT8Z[]), workouts_supported(21,UINT32Z), connectivity_supported(23,UINT32Z) — битовая маска GarminCapability (FitLocalMessageHandler.java:92-97), wifi(24), audio_prompts(26).
- USER_PROFILE(3): friendly_name(0,STRING[8]), gender(1), age(2), height, weight + sleep_time / wake_time (SleepTest).
- TIME_IN_ZONE(216): time_in_zone(2,UINT32,HR_TIME_IN_ZONE), hr_zone_high_boundary(6,UINT8[]), max/resting/threshold_heart_rate(11..13), functional_threshold_power(15).

### 5.17 FILETYPE (значение file_id.type) — service/devices/garmin/FileType.java:36-155
Значение = subtype при type=128: SETTINGS=2, SPORTS=3, ACTIVITY=4, WORKOUTS=5, COURSES=6, SCHEDULES=7, LOCATION=8, WEIGHT=9, TOTALS=10, GOALS=11, MONITOR_A=15, MONITOR_DAILY=28, MONITOR=32, SEGMENT_LIST=35, SCORE=38, CHANGELOG=41, METRICS=44, HSA_DATA=48, SLEEP=49, USER_BEHAVIOR_LOG=52, SPORTS_BACKUP=57, DEVICE_58=58, ECG=61, HRV_STATUS=68, HSA=70, COM_ACT=71, FBT_BACKUP=72, SKIN_TEMP=73, FBT_PTD_BACKUP=74, SCHEDULE=77, SLP_DISR=79, AREA_COURSES=82, GEAR=87. Не-FIT: DIRECTORY(0,0), DEVICE_XML(8,255), PRG(255,17), ERROR_SHUTDOWN_REPORTS(255,245), IQ_ERROR_REPORTS(255,244), GOLF_SCORECARD(255,246), ULF_LOGS(255,247), KPI(255,248). Флаг pull = «скачивать с часов».

## 6. Импорт в БД (fit/FitImporter.java)

### 6.1 Диспетчеризация по типу файла (FitImporter.java:560-620)

| FILETYPE | Что сохраняется |
|---|---|
| ACTIVITY | persistWorkout -> BaseActivitySummary + GenericMetricSample; экспорт GPX/FIT |
| MONITOR | GarminActivitySample + GarminIntensityMinutesSample; GarminSpo2Sample, GarminRespiratoryRateSample, GarminHeartRateRestingSample, GarminStressSample, GarminBodyEnergySample, GarminRestingMetabolicRateSample |
| METRICS | GenericTrainingLoadAcuteSample, GenericTrainingLoadChronicSample |
| SLEEP | GarminEventSample, GarminSleepStatsSample, GarminNapSample, GarminSleepRestlessMomentsSample, GarminSleepStageSample (условно), processRawSleepSamples() |
| HRV_STATUS | GarminHrvSummarySample, GarminHrvValueSample |
| прочее | WARN "Unable to handle fit file of type" |
| ВСЕГДА | BatteryLevel + GenericMetricSample (второй проход, :622-634) |

Перед записью файл копируется в экспорт-директорию: [TYPE]/[yyyy]/[TYPE]_[yyyy-MM-dd_HH-mm-ss].fit (GarminUtils.buildExportPath, GarminUtils.java:43-63; FitImporter.getFilePath :950-959).

### 6.2 Таблицы (GreenDAO, GBDaoGenerator/src/.../GBDaoGenerator.java:1113-1262)

| Сущность | Поля (сверх timestamp/deviceId/userId) | PK |
|---|---|---|
| GarminActivitySample | rawIntensity, steps, rawKind, heartRate, distanceCm, activeCalories | (timestamp СЕК, deviceId, userId) |
| GarminStressSample | stress | (timestamp МС, deviceId, userId) |
| GarminBodyEnergySample | energy | - |
| GarminSpo2Sample | spo2, typeNum | - |
| GarminSleepStageSample | stage (0..4 = SleepStage.num) | - |
| GarminEventSample | event (в PK), eventType, data | (ts, dev, user, event) |
| GarminHrvSummarySample | weeklyAverage, lastNightAverage, lastNight5MinHigh, baselineLowUpper, baselineBalancedLower, baselineBalancedUpper, statusNum | - |
| GarminHrvValueSample | value (мс) | - |
| GarminRespiratoryRateSample | respiratoryRate (float) | - |
| GarminHeartRateRestingSample | heartRate | - |
| GarminRestingMetabolicRateSample | restingMetabolicRate | - |
| GarminSleepStatsSample | sleepScore | - |
| GarminIntensityMinutesSample | moderate, vigorous | - |
| GarminNapSample | endTimestamp | - |
| GarminSleepRestlessMomentsSample | count | - |
| GarminFitFile | downloadTimestamp, deviceId, userId, fileNumber, fileDataType, fileSubType, fileTimestamp, specificFlags, fileSize, fileData(blob); UNIQUE(deviceId,userId,fileNumber) | id autoincrement |
| GenericMetricSample | metric (MetricSample.Metric), value (double), level (long) | - |
| BatteryLevel | batteryIndex, level; timestamp в СЕКУНДАХ | - |

Единицы: GarminActivitySample.timestamp — секунды; все AbstractTimeSample-наследники — миллисекунды (sample.setTimestamp(ts * 1000L)). Дедупликация: persistSamples (devices/AbstractSampleProvider.java:720-758) проставляет deviceId/userId и делает insertOrReplaceInTx -> upsert по составному PK. Логики слияния нет.

### 6.3 Агрегация шагов/дистанции/калорий (persistActivitySamples, :755-925)

    activitySamplesPerTimestamp: SortedMap<unixSec, []FitMonitoring>   // ключ = computeTimestamp()
    THRESHOLD_NOT_WORN = 600 с
    для каждого ts по возрастанию:
      если ts - prevTs > 60: заполнить пропуск сэмплами каждые 60 с,
          rawKind = (ts-prevTs > 600) ? NOT_WORN : prevActivityKind, остальное NOT_MEASURED
      sample.rawKind = ACTIVITY, остальное NOT_MEASURED
      для каждой FitMonitoring этой секунды:
        activityType = activity_type(5) ?? (current_activity_type_intensity & 0x1F) ?? NOT_MEASURED
        heart_rate(27)             -> sample.heartRate (побеждает последнее)
        cycles(3)                  -> stepsPerActivity[activityType]    = v   (ЗАМЕНА, не +=)
        distance(2)                -> distancePerActivity[activityType] = v
        active_calories(19)        -> caloriesPerActivity[activityType] = v
        (cur_act_type_intensity >> 5) & 0x7 -> sample.rawIntensity
        moderate_activity_minutes(33) -> minutesModerate += v
        vigorous_activity_minutes(34) -> minutesVigorous += v
      sample.steps          = сумма stepsPerActivity.values()
      sample.distanceCm     = сумма distancePerActivity.values()
      sample.activeCalories = сумма caloriesPerActivity.values()
      полностью пустые сэмплы отбрасываются
      minutesModerate|minutesVigorous != 0 -> GarminIntensityMinutesSample(ts*1000, moderate, vigorous)

Garmin шлёт кумулятивные значения на каждый тип активности; карты *PerActivity живут в пределах одного файла и суммируются на каждом сэмпле.

### 6.4 Сырые сны (processRawSleepSamples, :905-960)
Если есть SLEEP_DATA_RAW, но нет ни одного SleepStage со stage != 0 && stage != 1: asleep = file_id.time_created * 1000; wake = asleep + N_raw * 60000; создаются два GarminEventSample: event=74, eventType=0/1, data=-1 (маркер синтетики); реальные sleepStageSamples НЕ сохраняются.

### 6.5 Тренировки (persistWorkout :646-726 + GarminWorkoutParser)
Начало = session.start_time, иначе file_id.time_created; при переимпорте summary ищется по time_created (идемпотентность). summary.rawDetailsPath = путь к скопированному .fit; детальный разбор ленивый. FitRecord -> ActivityPoint (трек); FitSession/FitLap/FitSet/FitTimeInZone/FitPhysiologicalMetrics/FitSport/FitUserProfile/FitUserMetrics/FitDive*/FitTankSummary/FitDeviceInfo/FitDeviceStatus/FitWorkout/FitActivity -> поля ActivitySummaryData. Многосессионные файлы: первая SESSION основная, остальные — только в таблицу (allSessions). Авто-экспорт: AutoGpxExporter, AutoFitExporter (FitImporter.java:715-724).

### 6.6 Конвейер
FitAsyncProcessor.process(files, isReprocessing, cb) — отдельный поток; при исключении файл НЕ удаляется из PendingFile, иначе PendingFileProvider.removePendingFile(path) (fit/FitAsyncProcessor.java:37-76).

## 7. Что приложение ПИШЕТ на часы

### 7.1 Генерация файла (FitFile(List<RecordData>), FitFile.java:57-62, 155-177)
Header: hasCRC=true (14 байт), protocolVersion=16, profileVersion=21117, dataSize вычисляется. Определение пишется только когда RecordDefinition отличается от предыдущего. Данные исходящих записей — BIG_ENDIAN (FitRecordDataBuilder.build :48-58); заголовок и CRC — LE. Оценка буфера: 14 + сумма(5 + 3*nFields + 1 + 1 + valueSize) + 2 (FitFile.getOutgoingMessage :200-217).

### 7.2 SETTINGS-файл с будильниками (GarminSupport.onSetAlarms, GarminSupport.java:1206-1297)

    FILE_ID(0):         type=SETTINGS(128/2), manufacturer=1 (garmin), product=65534 (connect),
                        time_created=now(unix), serial_number=1, number=1
    ALARM_SETTINGS(222) x N: time=LocalTime(hh,mm), repeat = alarm.repetition ?: 128,
                        enabled=0/1, sound = 0 OFF / 1 TONE / 2 VIBRATION / 3 TONE_AND_VIBRATION,
                        backlight=0/1, time_created=now, snooze=0, label=AlarmLabel, message_index=i
    DEVICE_SETTINGS(2) (если N>0): alarms_time[] (минуты от полуночи), alarms_unk5[] (все = 5),
                        alarms_enabled[], alarms_repeat[]

Отправка: fileTransferHandler.initiateUpload(fitBytes, fileType).

### 7.3 SETTINGS-файл SleepTest (devices/garmin/GarminSettingsCustomizer.java:290-337)

    FILE_ID(0): type=SETTINGS, serial=1, time_created=now, manufacturer=1, product=65534,
                number=1, product_name="GBSleepTest"
    USER_PROFILE(3): wake_time = сек от полуночи, sleep_time = сек от полуночи

Передаётся как «установка приложения» (onInstallApp с BUNDLE_EXTRA_INSTALL_BYTES).

### 7.4 Погода — НЕ файл, а локальные FIT-сообщения поверх GFDI
GarminSupport.sendWeatherConditions / encodeWeather (GarminSupport.java:737-849): нужна capability WEATHER_CONDITIONS, иначе погода отдаётся по HTTP-перехвату. FitLocalMessageBuilder, максимум 15 локальных типов (FitLocalMessageBuilder.java:12): localType0 — WEATHER(128) weather_report=current (temperature, low/high, condition, wind_direction, precip_prob, wind_speed, feels_like, humidity, observed lat/long, air_quality, dew_point, location, observed_at_time, timestamp); localType1 — до 12 записей hourly_forecast (temp, condition, wind dir/speed, precip, feels_like, humidity, dew_point, uv_index, air_quality=null, atmospheric_pressure); localType2 — «сегодня» + до 4 daily_forecast (low/high, condition, precip, day_of_week, aqi, humidity, wind, uv, pressure). Все записи одного localType обязаны иметь одинаковый набор полей — иначе builder пересобирает запись под зарегистрированное определение (:86-105).
Преобразования FitWeather.Builder (messages/FitWeather.java:37-170): температура °C = K - 273 (намеренно «неточно», #4313); ветер мм/с = round(км/ч / 3.6 * 1000), клэмп 0xFFFE; направление deg % 360, отрицательное -> null; вероятность осадков и влажность клэмп 0..100; UV клэмп 0..10 (без масштабирования); давление Па = round(мбар * 100); day_of_week из системной таймзоны; условие: OWM-код -> WeatherCondition (255 если нет соответствия). Отправка: FitLocalMessageHandler.init() шлёт definition-сообщения, затем данные.

### 7.5 Курсы из GPX
fit/GpxRouteFileConverter.java собирает FIT типа COURSES: FILE_ID, COURSE(31), LAP(19), EVENT(21), RECORD(20), COURSE_POINT(32) -> FitFile(courseFileDataRecords) (:213-215) -> upload.

### 7.6 Экспорт активности
export/FitExporter.java:500-511 — FitFile(records) из трека, запись в поток (авто-экспорт .fit).

### 7.7 AGPS — НЕ FIT
Эфемериды отдаются через перехват HTTP-запросов часов к api.gcs.garmin.com/ephemeris/... (service/devices/garmin/http/interceptors/AgpsInterceptor.java): локальный файл отдаётся как есть, ETag = md5. Валидация (agps/GarminAgpsFile.java): tar с CPE_GLO.BIN / CPE_QZSS.BIN / CPE_GPS.BIN / CPE_GAL.BIN (agps/GarminAgpsDataType.java:3-6); gzip «rxnetworks» с сигнатурой CPE_RXNETWORKS_HEADER и BE-uint32 timestamp (годен <= 604800 с); Sony CPE. Статусы: MISSING/PENDING/CURRENT/ERROR.

## 8. Подводные камни для Go-порта

1. Профиль обязателен: без fit_profile.json нет имён/scale/offset/enum-типов — парсер деградирует до «номер поля -> сырое число».
2. Порядок байт разный по записям: definition задаёт arch для своего сообщения; header/CRC всегда LE; исходящие записи в этом коде BE.
3. Compressed timestamp обновляет referenceTimestamp ДО разбора тела и работает только с localMessageType 0..3.
4. timestamp16 в MONITORING требует rollover-математики (раздел 4) — иначе шаги «съезжают» на часы.
5. nil-значение поля = «нет данных»; в БД пишется ActivitySample.NOT_MEASURED.
6. getFieldByNumber учитывает приоритет dev-полей с native_mesg_num/native_field_num (CIQ).
7. Мягкая обработка расхождений типов (ENUM/UINT8, UINT32Z/UINT32, size=1 при UINT16/32/64) обязательна для реальных прошивок/COROS.
8. CIQ-значения иногда приходят массивом там, где ожидается скаляр — берётся [0] (RecordData.java:194-205).
9. Склеенные в один поток FIT-файлы НЕ поддерживаются (только WARN).