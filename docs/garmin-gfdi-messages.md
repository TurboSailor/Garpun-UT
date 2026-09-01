# GFDI: протокол сообщений Garmin (извлечено из Pulse/Gadgetbridge)

Пути — относительно app/src/main/java/nodomain/freeyourgadget/gadgetbridge/. Пакет сообщений: service/devices/garmin/messages/ (далее msg/), статусы — msg/status/.
Endianness: little-endian ВЕЗДЕ (MessageReader и MessageWriter выставляют LITTLE_ENDIAN) — msg/GFDIMessage.java:159, msg/MessageWriter.java:20.

## 0. Общий кадр GFDI

    offset size type    name
    0      2    uint16  packetSize   длина кадра ВКЛЮЧАЯ эти 2 байта и CRC
    2      2    uint16  messageType  GFDI message ID
    4      N    bytes   payload
    4+N    2    uint16  crc          CRC-16 Garmin/ANT по байтам [0, packetSize-2)

Проверки при приёме (msg/GFDIMessage.java:157-185): packetSize == длине буфера; CRC читается как uint16 по смещению packetSize-2 и сверяется с computeCrc(buf,0,packetSize-2); limit ставится в packetSize-2 (парсеры CRC не видят).
Сборка исходящего (msg/GFDIMessage.java:56-84): placeholder 0x0000, затем ID и payload; в конце putShort(0, position()+2) и дописывается CRC. То есть: сначала тело, потом длина = len(body)+2, потом CRC.

### CRC (nibble-табличный, ChecksumCalculator.java:22-52)

    var crcTable = [16]uint16{
      0x0000, 0xCC01, 0xD801, 0x1400, 0xF001, 0x3C00, 0x2800, 0xE401,
      0xA001, 0x6C00, 0x7800, 0xB401, 0x5000, 0x9C01, 0x8801, 0x4400,
    }
    func crc16(initial uint16, data []byte) uint16 {
        crc := initial
        for _, b := range data {
            crc = ((crc >> 4) & 0x0FFF) ^ crcTable[crc&0x0F] ^ crcTable[b&0x0F]
            crc = ((crc >> 4) & 0x0FFF) ^ crcTable[crc&0x0F] ^ crcTable[(b>>4)&0x0F]
        }
        return crc
    }

Тот же CRC — как running CRC файловых передач и передач уведомлений (initial = CRC всех предыдущих чанков).

### Диспетчеризация типа (msg/GFDIMessage.java:26-52)

    messageType = readShort();
    if ((messageType & 0x8000) != 0) {           // сокращённая/секвенированная форма
        // sequenceNumber = (messageType >> 8) & 0x7f;  — в коде закомментировано, не используется
        messageType = (messageType & 0xff) + 5000;
    }

Неизвестный ID -> UnhandledMessage -> ответ RESPONSE(UNSUPPORTED).
Чтение строк: 1 байт длины + UTF-8 (readString) либо NUL-terminated (readNullTerminatedString) — GarminByteBufferReader.java:60-82.

## 1. Таблица GFDI message ID (msg/GFDIMessage.java:86-118)

| ID | enum | класс | назначение |
|----|------|-------|-----------|
| 5000 | RESPONSE | status/GFDIStatusMessage | статус/ACK на любое сообщение, обе стороны |
| 5002 | DOWNLOAD_REQUEST | DownloadRequestMessage | Phone->Watch: скачивание файла по fileIndex |
| 5003 | UPLOAD_REQUEST | UploadRequestMessage | Phone->Watch: заливка в созданный файл |
| 5004 | FILE_TRANSFER_DATA | FileTransferDataMessage | обе стороны: чанк данных файла |
| 5005 | CREATE_FILE | CreateFileMessage | Phone->Watch: создать файл типа/размера |
| 5007 | FILTER | FilterMessage | Phone->Watch: фильтр перед листингом директории |
| 5008 | SET_FILE_FLAG | SetFileFlagsMessage | Phone->Watch: пометить файл ARCHIVE/DELETE |
| 5009 | FILE_AVAILABLE | FileAvailableMessage | Watch->Phone: новый файл (Pulse отвечает UNSUPPORTED) |
| 5011 | FIT_DEFINITION | FitDefinitionMessage | обе стороны: FIT record definitions |
| 5012 | FIT_DATA | FitDataMessage | обе стороны: FIT record data (realtime, погода, capabilities) |
| 5014 | WEATHER_REQUEST | WeatherMessage | Watch->Phone: запрос погоды |
| 5023 | BATTERY_STATUS | BatteryStatusMessage | Watch->Phone: статус батареи (только лог) |
| 5024 | DEVICE_INFORMATION | DeviceInformationMessage | Watch->Phone: инфо устройства; ответ RESPONSE с нашим инфо |
| 5026 | DEVICE_SETTINGS | SetDeviceSettingsMessage | Phone->Watch: набор настроек |
| 5030 | SYSTEM_EVENT | SystemEventMessage | Phone->Watch: системные события |
| 5031 | SUPPORTED_FILE_TYPES_REQUEST | SupportedFileTypesMessage | Phone->Watch: список поддерживаемых типов файлов |
| 5033 | NOTIFICATION_UPDATE | NotificationUpdateMessage | Phone->Watch: add/modify/remove уведомления |
| 5034 | NOTIFICATION_CONTROL | NotificationControlMessage | Watch->Phone: запрос атрибутов / action |
| 5035 | NOTIFICATION_DATA | NotificationDataMessage | Phone->Watch: чанк данных уведомления |
| 5036 | NOTIFICATION_SUBSCRIPTION | NotificationSubscriptionMessage | Watch->Phone: вкл/выкл подписки |
| 5037 | SYNCHRONIZATION | SynchronizationMessage | Watch->Phone: часы просят синк (битмаска) |
| 5039 | FIND_MY_PHONE_REQUEST | FindMyPhoneRequestMessage | Watch->Phone: найти телефон |
| 5040 | FIND_MY_PHONE_CANCEL | FindMyPhoneCancelMessage | Watch->Phone: отмена |
| 5041 | MUSIC_CONTROL | MusicControlMessage | Watch->Phone: команда плееру |
| 5042 | MUSIC_CONTROL_CAPABILITIES | MusicControlCapabilitiesMessage | Watch->Phone: запрос возможностей |
| 5043 | PROTOBUF_REQUEST | ProtobufMessage | обе стороны: protobuf-запрос GdiSmartProto.Smart |
| 5044 | PROTOBUF_RESPONSE | ProtobufMessage | обе стороны: protobuf-ответ |
| 5049 | MUSIC_CONTROL_ENTITY_UPDATE | MusicControlEntityUpdateMessage | Phone->Watch: метаданные плеера/очереди/трека |
| 5050 | CONFIGURATION | ConfigurationMessage | Watch->Phone: capabilities-битфилд; в ответ шлём свой |
| 5052 | CURRENT_TIME_REQUEST | CurrentTimeRequestMessage | Watch->Phone: запрос времени/TZ |
| 5101 | AUTH_NEGOTIATION | AuthNegotiationMessage | Watch->Phone: согласование аутентификации |

## 2. Status/Response (ID 5000) — механика

msg/status/GFDIStatusMessage.java:9-52. Общий приём:

    0 2 uint16 originalMessageType
    2 1 uint8  status (GFDIMessage.Status)
    3 ...      хвост, зависящий от originalMessageType

Сопоставление запрос/ответ идёт ТОЛЬКО по originalMessageType (плюс requestId внутри protobuf). Глобального sequence-номера в реализации нет.

enum Status (ordinal = код), msg/GFDIMessage.java:120-138: 0 ACK, 1 NAK, 2 UNSUPPORTED, 3 DECODE_ERROR, 4 CRC_ERROR, 5 LENGTH_ERROR.

Ветвление парсера: PROTOBUF_REQUEST/RESPONSE -> ProtobufStatusMessage; NOTIFICATION_DATA -> NotificationDataStatusMessage; UPLOAD_REQUEST -> UploadRequestStatusMessage; DOWNLOAD_REQUEST -> DownloadRequestStatusMessage; FILE_TRANSFER_DATA -> FileTransferDataStatusMessage; CREATE_FILE -> CreateFileStatusMessage; SUPPORTED_FILE_TYPES_REQUEST -> SupportedFileTypesStatusMessage; SET_FILE_FLAG -> SetFileFlagsStatusMessage; FIT_DEFINITION -> FitDefinitionStatusMessage; FIT_DATA -> FitDataStatusMessage; AUTH_NEGOTIATION -> AuthNegotiationStatusMessage; FILTER -> FilterStatusMessage; иначе generic (только status), ответ на ACK не шлётся. Почти все специализированные парсеры при status != ACK возвращают null.

Generic ACK (status/GenericStatusMessage.java:29-37):

    0 2 uint16 packetSize
    2 2 uint16 5000
    4 2 uint16 originalMessageType
    6 1 uint8  status
    7 2 uint16 crc

По умолчанию каждое входящее сообщение отвечает GenericStatusMessage(ACK) (GFDIMessage.java:71); UnhandledMessage и FileAvailableMessage — UNSUPPORTED.

Порядок обработки входящего (GarminSupport.java:371-419): 1) прогон через цепочку MessageHandler (fileTransferHandler -> protocolBufferHandler -> notificationsHandler -> динамические FitLocalMessageHandler), первый ненулевой = followup; 2) ACK/статус входящего; 3) reply самого сообщения (если generateOutgoing()==true); 4) followup; 5) GBDeviceEvent-ы; 6) processDownloadQueue().

DownloadRequestStatusMessage (status/DownloadRequestStatusMessage.java:15-29):

    0 1 uint8 status (должен быть ACK)
    1 1 uint8 downloadStatus
    2 4 int32 maxFileSize   реальный размер файла

DownloadStatus: 0 OK, 1 INDEX_UNKNOWN, 2 INDEX_NOT_READABLE, 3 NO_SPACE_LEFT, 4 INVALID, 5 NOT_READY, 6 CRC_INCORRECT.

UploadRequestStatusMessage (status/UploadRequestStatusMessage.java:19-36):

    0  1 uint8  status
    1  1 uint8  uploadStatus
    2  4 int32  dataOffset
    6  4 int32  maxFileSize
    10 2 uint16 crcSeed

UploadStatus: 0 OK, 1 INDEX_UNKNOWN, 2 INDEX_NOT_WRITEABLE, 3 NO_SPACE_LEFT, 4 INVALID, 5 NOT_READY, 6 CRC_INCORRECT.

FileTransferDataStatusMessage (status/FileTransferDataStatusMessage.java:23-36, 47-57):
приём: 0:1 status, 1:1 transferStatus, 2:4 int32 dataOffset (сколько байт принято).
отправка (подтверждение чанка):

    0 2 uint16 packetSize
    2 2 uint16 5000
    4 2 uint16 5004
    6 1 uint8  status = ACK(0)
    7 1 uint8  transferStatus = OK(0)
    8 4 int32  dataOffset = входящий.dataOffset + len(chunk)

TransferStatus: 0 OK, 1 RESEND, 2 ABORT, 3 CRC_MISMATCH, 4 OFFSET_MISMATCH, 5 SYNC_PAUSED.

CreateFileStatusMessage (status/CreateFileStatusMessage.java:22-38):

    0 1 uint8  status
    1 1 uint8  createStatus
    2 2 uint16 fileIndex   индекс созданного файла для UPLOAD_REQUEST
    4 1 uint8  dataType
    5 1 uint8  subType
    6 2 uint16 fileNumber

CreateStatus: 0 OK, 1 DUPLICATE, 2 NO_SPACE, 3 UNSUPPORTED, 4 NO_SLOTS, 5 NO_SPACE_FOR_TYPE.

SetFileFlagsStatusMessage (status/SetFileFlagsStatusMessage.java:22-38): 0:1 status, 1:1 flagsStatus (0 APPLIED, 1 ERROR), 2:2 uint16 fileIdentifier (в коде readShort()+1), 4:1 uint8 fileFlags bitmask.

SupportedFileTypesStatusMessage (status/SupportedFileTypesStatusMessage.java:22-43):

    0 1 uint8 status
    1 1 uint8 typeCount
    далее typeCount раз: 1 uint8 fileDataType, 1 uint8 fileSubType, 1 uint8 nameLen, nameLen bytes UTF-8 (FIT_TYPE_4, ErrorShutdownReports, ...)

FilterStatusMessage (status/FilterStatusMessage.java:15-24): 0:1 status, 1:1 unk. Приход этого ACK — триггер initiateDownload().

FitDefinitionStatusMessage / FitDataStatusMessage: 0:1 status, 1:1 code. FitDefinition: 0 APPLIED, 1 NOT_UNIQUE, 2 OUT_OF_RANGE, 3 NOT_READY. FitData: 0 APPLIED, 1 NO_DEFINITION, 2 MISMATCH, 3 NOT_READY. Исходящая форма: [packetSize][5000][origId][status][code].

NotificationDataStatusMessage: 0:1 status, 1:1 transferStatus (0 OK, 1 RESEND, 2 ABORT, 3 CRC_MISMATCH, 4 OFFSET_MISMATCH).

NotificationControlStatusMessage (исходящий): [packetSize][5000][5034][status ACK][chunkStatus 0 OK/1 ERROR][code 0 NO_ERROR / 160 UNKNOWN_COMMAND].

NotificationSubscriptionStatusMessage (исходящий): [packetSize][5000][5036][status ACK][notificationStatus 0 ENABLED/1 DISABLED][enableRaw эхо][unk 0].

AuthNegotiationStatusMessage: приём 0:1 status, 1:1 authNegotiationStatus (0 GUESS_OK, 1 GUESS_KO), 2:1 unk, 3:4 int32 authFlags; отправка [packetSize][5000][5101][ACK][GUESS_OK=0][unk эхо][int32 flags эхо].

## 3. Побайтовые layout-ы базового цикла

### 5024 DEVICE_INFORMATION (msg/DeviceInformationMessage.java:55-101)
Приём:

    0  2 uint16 protocolVersion
    2  2 uint16 productNumber
    4  4 uint32 unitNumber
    8  2 uint16 softwareVersion   -> строка ver/100 . ver%100 (формат %d.%02d)
    10 2 uint16 maxPacketSize     -> лимит чанков FILE_TRANSFER_DATA
    12 .. string bluetoothFriendlyName (1 байт длины + UTF-8)
       .. string deviceName
       .. string deviceModel

Ответ (обязателен, иначе часы не считают телефон подключённым):

    0  2 uint16 packetSize
    2  2 uint16 5000
    4  2 uint16 5024
    6  1 uint8  status = ACK(0)
    7  2 uint16 ourProtocolVersion = 150
    9  2 uint16 ourProductNumber   = -1 -> 0xFFFF
    11 4 int32  ourUnitNumber      = -1 -> 0xFFFFFFFF
    15 2 uint16 ourSoftwareVersion = 7791
    17 2 uint16 ourMaxPacketSize   = -1 -> 0xFFFF
    19 .. string bluetoothName (пусто -> Gadgetbridge)
       .. string manufacturer
       .. string device
       1 uint8  protocolFlags = (incomingProtocolVersion/100 == 1) ? 1 : 0

События: версия ПО + MaxPacketSizeDeviceEvent -> fileTransferHandler.setMaxPacketSize (GarminSupport.java:495-499).

### 5050 CONFIGURATION (msg/ConfigurationMessage.java:27-43)
Приём: 0:1 uint8 numBytes, далее numBytes байт битфилда.
Ответ — тем же ID 5050 (не RESPONSE):

    0 2 uint16 packetSize
    2 2 uint16 5050
    4 1 uint8  len(payload) = 15 (120 бит)
    5 N bytes  наш битфилд

Бит i = байт i/8, бит i%8 (LSB-first) = capability с ordinal i (devices/garmin/GarminCapability.java:166-208). Ключевые ordinal: 3 SYNC, 4 DEVICE_INITIATES_SYNC, 5 HOST_INITIATED_SYNC_REQUESTS, 6 GNCS, 7 ADVANCED_MUSIC_CONTROLS, 8 FIND_MY_PHONE, 9 FIND_MY_WATCH, 10 CONNECTIQ_HTTP, 26 WEATHER_CONDITIONS, 27 WEATHER_ALERTS, 28 GPS_EPHEMERIS_DOWNLOAD, 29 EXPLICIT_ARCHIVE, 33 TRUEUP, 49 CALENDAR, 51 SMS_NOTIFICATIONS, 52 BASIC_MUSIC_CONTROLS, 55 GARMIN_DEVICE_INFO_FILE_TYPE, 68 EXPLORE_SYNC, 70 CURRENT_TIME_REQUEST_SUPPORT, 71 CONTACTS_SUPPORT, 76 MULTI_LINK_SERVICE, 77 OAUTH_CREDENTIALS, 92 REALTIME_SETTINGS. Всего 120 значений (0 CONNECT_MOBILE_FIT_LINK .. 119 UNK_119). Объявляем все, кроме UNK_104..UNK_111 и UNK_114..UNK_119 (GarminCapability.java:143-160). Приход 5050 -> CapabilitiesDeviceEvent -> completeInitialization().

### 5030 SYSTEM_EVENT (msg/SystemEventMessage.java:15-28)

    0 2 uint16 packetSize
    2 2 uint16 5030
    4 1 uint8  eventType
    5 ...      value: Integer -> 1 байт; String -> 1 байт длины + UTF-8

GarminSystemEventType (ordinal): 0 SYNC_COMPLETE, 1 SYNC_FAIL, 2 FACTORY_RESET, 3 PAIR_START, 4 PAIR_COMPLETE, 5 PAIR_FAIL, 6 HOST_DID_ENTER_FOREGROUND, 7 HOST_DID_ENTER_BACKGROUND, 8 SYNC_READY, 9 NEW_DOWNLOAD_AVAILABLE, 10 DEVICE_SOFTWARE_UPDATE, 11 DEVICE_DISCONNECT, 12 TUTORIAL_COMPLETE, 13 SETUP_WIZARD_START, 14 SETUP_WIZARD_COMPLETE, 15 SETUP_WIZARD_SKIPPED, 16 TIME_UPDATED. В Pulse value всегда 0 (1 байт). Использование: SYNC_READY при инициализации, PAIR_COMPLETE/SYNC_COMPLETE/SETUP_WIZARD_COMPLETE при первом коннекте (GarminSupport.java:857-879), SYNC_COMPLETE в конце синка (:1034) и после аплоада (FileTransferHandler.java:388), TIME_UPDATED в onSetTime (:1069), FOREGROUND/BACKGROUND по интентам (:189,192).

### 5026 DEVICE_SETTINGS (msg/SetDeviceSettingsMessage.java:19-40)

    0 2 uint16 packetSize
    2 2 uint16 5026
    4 1 uint8  settingsCount (1..255)
    для каждой настройки:
      1 uint8 settingId (ordinal)
      String  -> 1 uint8 len + UTF-8
      Integer -> 1 uint8 0x04 + 4 int32
      Boolean -> 1 uint8 0x01 + 1 uint8 (0/1)

GarminDeviceSetting (ordinal): 0 DEVICE_NAME, 1 CURRENT_TIME, 2 DAYLIGHT_SAVINGS_TIME_OFFSET, 3 TIME_ZONE_OFFSET, 4 NEXT_DAYLIGHT_SAVINGS_START, 5 NEXT_DAYLIGHT_SAVINGS_END, 6 AUTO_UPLOAD_ENABLED, 7 WEATHER_CONDITIONS_ENABLED, 8 WEATHER_ALERTS_ENABLED. Pulse шлёт ровно: AUTO_UPLOAD_ENABLED=true, WEATHER_CONDITIONS_ENABLED=true, WEATHER_ALERTS_ENABLED=false (GarminSupport.java:1059-1064).

### 5052 CURRENT_TIME_REQUEST (msg/CurrentTimeRequestMessage.java:22-89)
Приём: 0:4 int32 referenceID. Ответ:

    0  2 uint16 packetSize
    2  2 uint16 5000
    4  2 uint16 5052
    6  1 uint8  ACK
    7  4 int32  referenceID (эхо)
    11 4 int32  garminTimestamp = unixSec - 631065600
    15 4 int32  timeZoneOffset (секунды, включая DST)
    19 4 int32  nextTransitionEndsGarminTs (0 если нет)
    23 4 int32  nextTransitionStartsGarminTs (0 если нет)

GARMIN_TIME_EPOCH = 631065600 (GarminTimeUtils.java:8). Если синк времени выключен — вместо ответа GenericStatusMessage(5052, UNSUPPORTED) (GarminSupport.java:385-389).

### 5031 SUPPORTED_FILE_TYPES_REQUEST (msg/SupportedFileTypesMessage.java:11-18)
Запрос — пустой payload: [packetSize][5031][crc]. Ответ — SupportedFileTypesStatusMessage.

### 5023 BATTERY_STATUS (msg/BatteryStatusMessage.java:32-46)

    0 1 uint8 wireStatus  ((wireStatus & 0x70): 0x20 good, 0x30 ok, 0x40 low)
    1 1 uint8 voltage_raw (V = raw/100.0)
    2..6      неизвестно

Процент заряда приходит НЕ здесь, а по protobuf GdiDeviceStatus.RemoteDeviceBatteryStatusResponse.currentBatteryLevel (ProtocolBufferHandler.java:400-410); подписка — RemoteDeviceBatteryStatusRequest при инициализации (GarminSupport.java:1046-1055).

### 5014 WEATHER_REQUEST (msg/WeatherMessage.java:19-27)

    0 1 uint8 format
    1 4 int32 latitude  (semicircles)
    5 4 int32 longitude (semicircles)
    9 1 uint8 hoursOfForecast

Ответ — generic ACK; погода отправляется как FIT (5011 + 5012), не отдельным сообщением (GarminSupport.java:736-749, encodeWeather). Нужна capability WEATHER_CONDITIONS, иначе часы заберут погоду по HTTP-протобуфу.

### 5037 SYNCHRONIZATION (msg/SynchronizationMessage.java:22-36)

    0 1 uint8 syncType (0 TYPE_0, 1 TYPE_1, 2 TYPE_2)
    1 1 uint8 bitmaskSize (8 или 4)
    2 N       bitmask: int64 LE при size==8, int32 при size==4

Биты: 1 SETTINGS, 2 GOALS, 3 WORKOUTS, 4 COURSES, 5 ACTIVITIES, 6 RECORDS, 8 SOFTWARE_UPDATE, 9 DEVICE_CONFIG, 11 USER, 12 SPORTS, 13 SEGMENTS, 17 INSTALL, 19 TRUE_UP, 21 ACTIVITY_SUMMARY, 22 METRICS, 23 PACE_BAND, 26 SLEEP (прочие unk_N). shouldProceed() = WORKOUTS or ACTIVITIES or ACTIVITY_SUMMARY or SLEEP. Тогда шлём FilterMessage; если идёт аплоад — откладываем до простоя (FileTransferHandler.java:120-130).

### 5007 FILTER (msg/FilterMessage.java:11-24)

    0 2 uint16 packetSize
    2 2 uint16 5007
    4 1 uint8  filterType = 3 (UNK_3)

FilterType: 0 NO_0, 1 UNK_1, 2 UNK_2, 3 UNK_3.

### 5039 / 5040 FIND MY PHONE
5039: 0:1 uint8 duration (сек) -> событие START, generic ACK (msg/FindMyPhoneRequestMessage.java:19-31). 5040: пустой payload -> STOP, generic ACK.

### 5041 MUSIC_CONTROL (msg/MusicControlMessage.java:39-43)
0:1 uint8 commandOrdinal. GarminMusicControlCommand: 0 TOGGLE_PLAY_PAUSE, 1 SKIP_TO_NEXT_ITEM, 2 SKIP_TO_PREVIOUS_ITEM, 3 VOLUME_UP, 4 VOLUME_DOWN, 5 PLAY, 6 PAUSE, 7 SKIP_FORWARD, 8 SKIP_BACKWARDS.

### 5042 MUSIC_CONTROL_CAPABILITIES (msg/MusicControlCapabilitiesMessage.java:14-36)
Приём: 0:1 uint8 supportedCapabilities (может отсутствовать). Ответ:

    0 2 uint16 packetSize
    2 2 uint16 5000
    4 2 uint16 5042
    6 1 uint8  ACK
    7 1 uint8  count = 9
    8 9 uint8[] ordinal-ы 0..8

### 5049 MUSIC_CONTROL_ENTITY_UPDATE (msg/MusicControlEntityUpdateMessage.java:18-69)
Повторяющиеся TLV:

    0 1 uint8 len = 3 + len(value)   (value <= 252 байт)
    1 1 uint8 entityId
    2 1 uint8 attributeOrdinal
    3 1 uint8 0 (назначение неизвестно)
    4 len-3   value UTF-8

entityId 0 PLAYER {0 NAME, 1 PLAYBACK_INFO, 2 VOLUME}; 1 QUEUE {0 INDEX, 1 COUNT, 2 SHUFFLE, 3 REPEAT}; 2 TRACK {0 ARTIST, 1 ALBUM, 2 TITLE, 3 DURATION}.

### 5033 NOTIFICATION_UPDATE (msg/NotificationUpdateMessage.java:31-41)

    0  2 uint16 packetSize
    2  2 uint16 5033
    4  1 uint8  updateType (0 ADD, 1 MODIFY, 2 REMOVE)
    5  1 uint8  categoryFlags (битмаска NotificationFlag)
    6  1 uint8  categoryValue (NotificationCategory ordinal)
    7  4 int32  notificationId
    11 1 uint8  phoneFlags (битмаска NotificationPhoneFlags)

NotificationFlag биты: 0 BACKGROUND(1), 1 FOREGROUND(2), 2 UNK(4), 3 ACTION_ACCEPT(8), 4 ACTION_DECLINE(16); практически всегда FOREGROUND|ACTION_DECLINE = 0x12. NotificationCategory (ordinal): 0 OTHER, 1 INCOMING_CALL, 2 MISSED_CALL, 3 VOICEMAIL, 4 SOCIAL, 5 SCHEDULE, 6 EMAIL, 7 NEWS, 8 HEALTH_AND_FITNESS, 9 BUSINESS_AND_FINANCE, 10 LOCATION, 11 ENTERTAINMENT, 12 SMS. NotificationPhoneFlags: 0 LEGACY_ACTIONS(1), 1 NEW_ACTIONS(2), 2 HAS_ATTACHMENTS(4).

### 5034 NOTIFICATION_CONTROL (msg/NotificationControlMessage.java:71-136)
0:1 uint8 command. NotificationCommand (коды, не ordinal): 0 GET_NOTIFICATION_ATTRIBUTES, 1 GET_APP_ATTRIBUTES, 2 PERFORM_LEGACY_NOTIFICATION_ACTION, 128 PERFORM_NOTIFICATION_ACTION (NotificationsHandler.java:308-318).
- command 0: 1:4 int32 notificationId, далее до конца: 1 uint8 attributeId; если атрибут hasLengthParam (TITLE=1, SUBTITLE=2, MESSAGE=3) — 2 uint16 maxLength; если hasAdditionalParams (ACTIONS=127) — 2 uint16 maxLength + 1 uint8 unk; иначе maxLength=0 (без ограничения). Коды атрибутов: 0 APP_IDENTIFIER, 1 TITLE, 2 SUBTITLE, 3 MESSAGE, 4 MESSAGE_SIZE, 5 DATE, 7 NEGATIVE_ACTION_LABEL, 127 ACTIONS, 128 ATTACHMENTS.
- command 2: 1:4 int32 notificationId, 5:1 uint8 action (0 ACCEPT, 1 REFUSE).
- command 128: 1:4 int32 notificationId, 5:1 uint8 actionCode, далее опционально NUL-terminated UTF-8 (текст ответа). Коды действий: 1..5 CUSTOM_ACTION_1..5, 94 REPLY_INCOMING_CALL, 95 REPLY_MESSAGES, 96 ACCEPT_INCOMING_CALL, 97 REJECT_INCOMING_CALL, 98 DISMISS_NOTIFICATION, 99 BLOCK_APPLICATION.
- command 1: NUL-terminated appIdentifier + список байт-кодов атрибутов (сейчас только 0 APP_NAME).
Ответ — NotificationControlStatusMessage, followup — 5035.

### 5035 NOTIFICATION_DATA (msg/NotificationDataMessage.java:27-37)

    0  2 uint16 packetSize
    2  2 uint16 5035
    4  2 uint16 messageSize  полный размер собираемого блока
    6  2 uint16 crc          running CRC по всем отправленным байтам включая текущий чанк
    8  2 uint16 dataOffset   смещение чанка (uint16!)
    10 N bytes  chunk        до 300 байт (maxBlockSize, NotificationsHandler.java:565)

Содержимое блока для GET_NOTIFICATION_ATTRIBUTES (NotificationsHandler.java:258-276): 1 uint8 command=0, 4 int32 notificationId, затем для каждого атрибута 1 uint8 attributeCode + 2 uint16 valueLen + значение UTF-8; MESSAGE_SIZE отправляется последним. Для GET_APP_ATTRIBUTES: 1 uint8 command=1, appIdentifier UTF-8, 0x00, затем attrCode + 2 uint16 len + значение.
Формат значения ACTIONS (NotificationsHandler.java:437-495): 1 uint8 actionCount, далее на действие 1 uint8 actionCode, 1 uint8 iconPositionBitmask (0 BOTTOM, 1 RIGHT, 2 LEFT), 1 uint8 descLen, descLen байт UTF-8. Если действий нет — 4 нулевых байта. DATE — строка формата yyyyMMdd T HHmmss (SimpleDateFormat yyyyMMdd'T'HHmmss).
Flow control: часы шлют NotificationDataStatusMessage, при OK шлём следующий чанк; по исчерпании — финальный NotificationDataStatusMessage(ACK, OK).

### 5036 NOTIFICATION_SUBSCRIPTION (msg/NotificationSubscriptionMessage.java:24-28)
0:1 uint8 enable (1 = вкл), 1:1 uint8 unk (может отсутствовать). Ответ — NotificationSubscriptionStatusMessage, формируется с учётом пользовательской настройки (GarminSupport.java:475-493).

### 5011 FIT_DEFINITION / 5012 FIT_DATA
5011 (msg/FitDefinitionMessage.java:36-44,57-65): payload — последовательность [RecordHeader 1 байт][RecordDefinition] в нативном FIT-формате до конца payload. Ответ — FitDefinitionStatusMessage.
5012 (msg/FitDataMessage.java:33-35,71-80): payload — сырые FIT data records [RecordHeader][поля], разбираются по ранее полученным definition (по localMessageType). Ответ — FitDataStatusMessage(ACK, APPLIED).
Исходящее чанкование (FitLocalMessageHandler.java:25,53-72): overhead кадра 6 байт (2 size + 2 id + 2 crc); records набираются пока 6 + sum(encodedSize) <= maxPacketSize. Последовательность: FIT_DEFINITION -> FitDefinitionStatus -> первый FIT_DATA -> на каждый FitDataStatus следующий чанк -> handler снимает себя.
Входящий FIT_DEFINITION -> IncomingFitDefinitionDeviceEvent -> регистрируется FitLocalMessageHandler, который разбирает входящие FIT_DATA (в т.ч. FIT_CAPABILITIES с полем connectivity_supported — 64-битная маска capabilities, FitLocalMessageHandler.java:92-99). Realtime-данные (шаги/сон/HR) приходят как FIT_DATA; поля — в отчёте по FIT-слою.

### 5101 AUTH_NEGOTIATION (msg/AuthNegotiationMessage.java:25-43)
Приём: 0:1 uint8 unk, 1:4 int32 authFlags. Ответ: AuthNegotiationStatusMessage(ACK, GUESS_OK, unk эхо, flags эхо). Собственное исходящее 5101 существует, но generateOutgoing() возвращает false.

## 4. Типы файлов (FileType.java:37-160)

Пара (type, subtype):
- виртуальные: DIRECTORY(0,0) — fileIndex всегда 0; UNKNOWN_1_0(1,0); DEVICE_XML(8,255) — fileIndex жёстко 0xFFFD (65533);
- FIT-файлы: type=128, subtype 1..99. Значимые: 2 SETTINGS, 4 ACTIVITY(pull), 5 WORKOUTS, 6 COURSES, 9 WEIGHT(pull), 15 MONITOR_A(pull), 28 MONITOR_DAILY(pull), 32 MONITOR(pull), 35 SEGMENT_LIST(pull), 38 SCORE(pull), 41 CHANGELOG(pull), 44 METRICS(pull), 49 SLEEP(pull), 52 USER_BEHAVIOR_LOG(pull), 57 SPORTS_BACKUP(pull), 58 DEVICE_58(pull), 61 ECG(pull), 66 FIT_TYPE_66(pull), 68 HRV_STATUS(pull), 70 HSA(pull), 71 COM_ACT(pull), 72 FBT_BACKUP(pull), 73 SKIN_TEMP(pull), 74 FBT_PTD_BACKUP(pull), 77 SCHEDULE(pull), 79 SLP_DISR(pull), 82 AREA_COURSES(pull);
- прочие: DOWNLOAD_COURSE(255,4), PRG(255,17), IQ_ERROR_REPORTS(255,244), ERROR_SHUTDOWN_REPORTS(255,245), GOLF_SCORECARD(255,246), ULF_LOGS(255,247), KPI(255,248).
isFitFile() == (type == 128). Флаг pull = скачивать при синке; остальные — только при включённой опции fetchUnknownFiles.

## 5. Файловые передачи

### 5.1 Листинг директории
1. Часы могут прислать SYNCHRONIZATION (5037); если shouldProceed(), телефон шлёт FILTER (5007, filterType=3).
2. FilterStatusMessage -> FileTransferHandler.initiateDownload() (FileTransferHandler.java:139-142): фиктивный DirectoryEntry(fileIndex=0, DIRECTORY) и DownloadRequestMessage(fileIndex=0, dataSize=0, NEW, crcSeed=0, dataOffset=0).
3. Тело директории — записи по 16 байт (длина обязана делиться на 16), парсинг FileTransferHandler.java:236-286:

    0  2 uint16 fileIndex
    2  1 uint8  fileDataType
    3  1 uint8  fileSubType
    4  2 uint16 fileNumber
    6  1 uint8  specificFlags
    7  1 uint8  fileFlags
    8  4 int32  fileSize
    12 4 int32  timestamp (Garmin epoch; 0 = нет даты)

Пропускаются: неизвестные (type,subtype); не-pull типы (если не включено fetchUnknownFiles); полностью нулевая запись (защита от бесконечного цикла).
4. DEVICE_XML качается вручную: DirectoryEntry(0xFFFD, DEVICE_XML, 0xFFFD, 0,0,0, now) (FileTransferHandler.java:144-146).

### 5.2 Скачивание файла
DownloadRequestMessage (msg/DownloadRequestMessage.java:26-37):

    0  2 uint16 packetSize
    2  2 uint16 5002
    4  2 uint16 fileIndex
    6  4 int32  dataOffset  (0 для нового)
    10 1 uint8  requestType (0 CONTINUE, 1 NEW)
    11 2 uint16 crcSeed     (0 для нового)
    13 4 int32  dataSize    (0 = весь файл)

Pulse всегда шлёт NEW, dataOffset=0, crcSeed=0, dataSize=0.
Ответ DownloadRequestStatusMessage даёт maxFileSize -> буфер ровно на этот размер (FileTransferHandler.java:429-436). Если canProceed()==false — FileDownloadedDeviceEvent{success=false}, очередь идёт дальше.
Затем часы шлют FILE_TRANSFER_DATA (5004), приём (msg/FileTransferDataMessage.java:31-38):

    0 1 uint8  flags
    1 2 uint16 crc         running CRC по всем принятым байтам файла включая этот чанк
    3 4 int32  dataOffset
    7 N bytes  данные до конца payload

Проверки (FileTransferHandler.java:438-449): dataOffset == позиции буфера; crc == computeCrc(runningCrc, chunk). На каждый чанк автоматически отправляется FileTransferDataStatusMessage(ACK, OK, dataOffset+len) — это и есть flow control (окно = 1 чанк). Файл завершён, когда буфер заполнен.
Завершение: DIRECTORY -> парсинг записей; иначе сохранение + FileDownloadedDeviceEvent{success, directoryEntry, localPath}. Путь экспорта: FILE_TYPE/YEAR/FILE_TYPE_yyyy-MM-dd_HH-mm-ss_INDEX.fit|bin (без даты — FILE_TYPE/FILE_TYPE_INDEX.ext), FileTransferHandler.java:517-528.

### 5.3 Пометка прочитанным / удаление (5008)
После успешного скачивания (и для уже скачанных ранее файлов) шлётся SetFileFlagsMessage(fileIndex, ARCHIVE), кроме DEVICE_XML и при включённой опции keep_activity_data_on_device (GarminSupport.java:524-525, :946-948).

    0 2 uint16 packetSize
    2 2 uint16 5008
    4 2 uint16 fileIndex
    6 1 uint8  flags bitmask

FileFlags (бит = 1<<ordinal): 0..3 UNK, 4 ARCHIVE = 0x10, 5 DELETE = 0x20 (msg/SetFileFlagsMessage.java:28-45).

### 5.4 Очередь скачивания
GarminSupport.processDownloadQueue() (GarminSupport.java:905-1015): одна активная закачка (currentlyDownloading), FIFO-очередь; уже скачанные пропускаются (только ARCHIVE). Когда очередь пуста — обработка накопленных FIT-файлов, затем finishFileSync() -> SYSTEM_EVENT(SYNC_COMPLETE).

### 5.5 Аплоад
1. CreateFileMessage (msg/CreateFileMessage.java:48-65):

    0  2 uint16 packetSize
    2  2 uint16 5005
    4  4 int32  fileSize
    8  1 uint8  dataType (filetype.type)
    9  1 uint8  subType  (filetype.subtype)
    10 2 uint16 fileIndex = 0
    12 1 uint8  reserved = 0
    13 1 uint8  subTypeMask = 0
    14 2 uint16 numberMask = 65535
    16 2 uint16 0 (неизвестно)
    18 8 int64  случайное число

Приём того же сообщения от часов короче: int32 fileSize, uint8 dataType, uint8 subType, uint16 fileIndex, uint8 unk, uint8 subTypeMask, uint16 numberMask (msg/CreateFileMessage.java:33-45).
2. CreateFileStatusMessage даёт fileIndex -> UploadRequestMessage (msg/UploadRequestMessage.java:32-42):

    0  2 uint16 packetSize
    2  2 uint16 5003
    4  2 uint16 fileIndex
    6  4 int32  size
    10 4 int32  dataOffset (0)
    14 2 uint16 crcSeed (0)

3. UploadRequestStatusMessage.canProceed() и совпадение dataOffset -> первый FILE_TRANSFER_DATA:

    0  2 uint16 packetSize
    2  2 uint16 5004
    4  1 uint8  flags = 0
    5  2 uint16 crc = running CRC после этого чанка
    7  4 int32  dataOffset
    11 N bytes  chunk, размер = min(remaining, maxPacketSize - 13)

(FileTransferHandler.java:451-458, msg/FileTransferDataMessage.java:53-62). maxPacketSize по умолчанию 375, обновляется из DEVICE_INFORMATION.
4. На каждый FileTransferDataStatusMessage(OK) с совпадающим offset — следующий чанк; когда dataSize <= dataOffset — аплоад завершён, шлётся SYSTEM_EVENT(SYNC_COMPLETE, 0) и берётся следующий upload (FileTransferHandler.java:375-403). Аплоады не перемежаются: очередь pendingUploads + runWhenUploadIdle (FileTransferHandler.java:313-341, :82-100).

## 6. Protobuf-слой (5043 / 5044)

Кадр (msg/ProtobufMessage.java:45-53, 79-89):

    0  2 uint16 packetSize
    2  2 uint16 5043 (REQUEST) | 5044 (RESPONSE)
    4  2 uint16 requestId
    6  4 int32  dataOffset
    10 4 int32  totalProtobufLength  размер ВСЕГО protobuf-сообщения
    14 4 int32  protobufDataLength   длина куска в этом кадре
    18 N bytes  фрагмент сериализованного GdiSmartProto.Smart

isChunked() = totalProtobufLength != protobufDataLength; isComplete() = dataOffset == 0 && !isChunked(). Целое сообщение подтверждается GenericStatusMessage(ACK); фрагмент — ProtobufStatusMessage(ACK, requestId, dataOffset, KEPT, NO_ERROR) (msg/ProtobufMessage.java:30-35).

ProtobufStatusMessage (status/ProtobufStatusMessage.java:30-45, 73-84):

    приём (после originalMessageType):
    0 1 uint8  status
    1 2 uint16 requestId
    3 4 int32  dataOffset
    7 1 uint8  chunkStatus (0 KEPT, 1 DISCARDED)
    8 1 uint8  statusCode
    отправка: [packetSize][5000][5043|5044][status][uint16 requestId][int32 dataOffset][chunkStatus][statusCode]

ProtobufStatusCode: 0 NO_ERROR, 100 UNKNOWN_REQUEST_ID, 101 DUPLICATE_PACKET, 102 MISSING_PACKET, 103 EXCEEDED_TOTAL_PROTOBUF_LENGTH, 200 PROTOBUF_PARSE_ERROR, 201 UNKNOWN. isOK() = ACK && KEPT && NO_ERROR.

Фрагментация (ProtocolBufferHandler.java): maxChunkSize = 375 (строка 74). Исходящее (:645-660): если len > 375 — весь массив в chunkedFragmentsMap[requestId], первый кадр dataOffset=0, totalProtobufLength=len, protobufDataLength=375; иначе один кадр. На каждый OK-статус следующий кусок: start = status.dataOffset + 375, length = min(375, len-start) (:725-733); когда totalLength <= dataOffset+375 — запись удаляется (:281-286). Входящее (:292-307): кадры с dataOffset==0 создают запись, последующие мержатся при toMerge.dataOffset == len(накопленного); готово при totalLength == len(fragmentBytes). requestId исходящих запросов: (last+1) % 65536 (:98-101); ответы используют requestId запроса.

Используемые сервисы GdiSmartProto.Smart (ProtocolBufferHandler.java:127-250): coreService (SyncResponse, GetLocationRequest, LocationUpdatedSetEnabledRequest), calendarService, smsNotificationService (canned lists), httpService (http/HttpHandler, ответ асинхронный через ProtobufResponseEvent), dataTransferService, deviceStatusService (battery/activity), findMyWatchService, settingsService (realtime settings: definition/state/change), authenticationService (OAuth — фейковые ключи), notificationsService, installedAppsService, fileSyncService (за флагом new_sync_protocol), ecgService, appConfigService, exploreSyncService (за флагом garmin_exploresync). Исходящие: RemoteDeviceBatteryStatusRequest (:1046-1055), SettingsService.InitRequest{language, region} при capability REALTIME_SETTINGS (:453-462), FindMyWatchService, FileSyncService.requestFile, canned messages.

## 7. Device events (мост протокол -> приложение), service/devices/garmin/deviceevents/

- MaxPacketSizeDeviceEvent(int) — из 5024, задаёт размер чанка файловых передач.
- CapabilitiesDeviceEvent(Set<GarminCapability>) — из 5050 (и из FIT_CAPABILITIES), триггерит completeInitialization().
- SupportedFileTypesDeviceEvent(List<FileType>) — из RESPONSE на 5031.
- NotificationSubscriptionDeviceEvent{enable} — из 5036.
- WeatherRequestDeviceEvent{format, latitude, longitude, hoursOfForecast} — из 5014.
- FileDownloadedDeviceEvent{success, directoryEntry, localPath} — завершение скачивания.
- IncomingFitDefinitionDeviceEvent(List<RecordDefinition>) — из 5011.
- ProtobufResponseEvent{payload, messageId} — асинхронный protobuf-ответ.

## 8. Порядок инициализации (что реализовать на Go)

1. Приходит 5024 DEVICE_INFORMATION -> отвечаем RESPONSE с нашей инфой (обязательно), запоминаем maxPacketSize.
2. Приходит 5050 CONFIGURATION -> отвечаем 5050 со своим capabilities-битфилдом.
3. completeInitialization() (GarminSupport.java:850-880): 5031 SUPPORTED_FILE_TYPES_REQUEST -> 5026 DEVICE_SETTINGS -> (если включён синк времени) 5030 TIME_UPDATED -> 5030 SYNC_READY -> protobuf RemoteDeviceBatteryStatusRequest; при первом коннекте дополнительно 5030 PAIR_COMPLETE, SYNC_COMPLETE, SETUP_WIZARD_COMPLETE.
4. Синк данных: 5002 DOWNLOAD_REQUEST(fileIndex=0) -> директория -> очередь -> на каждый файл 5002 + приём 5004 (+ автоматические ACK со смещением) + 5008 ARCHIVE -> в конце 5030 SYNC_COMPLETE.
5. Постоянно обслуживаем входящие: 5052 (время), 5014 (погода через FIT), 5034/5036 (уведомления), 5041/5042/5049 (музыка), 5039/5040 (find my phone), 5043/5044 (protobuf), 5037/5007 (синк-цикл), 5011/5012 (FIT).