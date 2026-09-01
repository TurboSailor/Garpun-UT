# Garmin: protobuf-сервисы, HTTP-прокси, AGPS, погода, уведомления, звонки, музыка, find-my

Разведка по `pulse-main`. Ссылки — `путь:строка`. Java-корень: `app/src/main/java/nodomain/freeyourgadget/gadgetbridge/service/devices/garmin/` (далее GARMIN/), протоколы — `app/src/main/proto/garmin/`.

---

## 0. Транспорт GFDI

GARMIN/messages/GFDIMessage.java

* Кадр (little-endian): `uint16 packetSize (включая CRC)` | `uint16 messageType` | payload | `uint16 CRC`. Сборка — addLengthAndChecksum(): putShort(0, position+2), затем CRC (ChecksumCalculator.computeCrc) по [0, position) — GFDIMessage.java:117-121.
* Разбор: MessageReader читает payloadSize, проверяет size==capacity и CRC, ставит limit = payloadSize-2 — GFDIMessage.java:160-190.
* Если `messageType & 0x8000 != 0` — status/response-форма: `messageType = (messageType & 0xff) + 5000`, биты 8..14 — sequence number — GFDIMessage.java:34-37.
* RESPONSE (5000): `uint16 5000 | uint16 originalMessageId | byte Status.ordinal() | доп. поля`.

### GarminMessage ID (GFDIMessage.java:89-121)

| ID | Имя |
|----|-----|
|5000|RESPONSE|
|5002|DOWNLOAD_REQUEST|
|5003|UPLOAD_REQUEST|
|5004|FILE_TRANSFER_DATA|
|5005|CREATE_FILE|
|5007|FILTER|
|5008|SET_FILE_FLAG|
|5009|FILE_AVAILABLE|
|5011|FIT_DEFINITION|
|5012|FIT_DATA|
|**5014**|**WEATHER_REQUEST**|
|5023|BATTERY_STATUS|
|5024|DEVICE_INFORMATION|
|**5026**|**DEVICE_SETTINGS**|
|5030|SYSTEM_EVENT|
|5031|SUPPORTED_FILE_TYPES_REQUEST|
|**5033**|**NOTIFICATION_UPDATE**|
|**5034**|**NOTIFICATION_CONTROL**|
|**5035**|**NOTIFICATION_DATA**|
|**5036**|**NOTIFICATION_SUBSCRIPTION**|
|5037|SYNCHRONIZATION|
|**5039**|**FIND_MY_PHONE_REQUEST**|
|**5040**|**FIND_MY_PHONE_CANCEL**|
|**5041**|**MUSIC_CONTROL**|
|**5042**|**MUSIC_CONTROL_CAPABILITIES**|
|**5043**|**PROTOBUF_REQUEST**|
|**5044**|**PROTOBUF_RESPONSE**|
|**5049**|**MUSIC_CONTROL_ENTITY_UPDATE**|
|5050|CONFIGURATION|
|5052|CURRENT_TIME_REQUEST|
|5101|AUTH_NEGOTIATION|

`Status` ordinal: ACK=0, NAK=1, UNSUPPORTED=2, DECODE_ERROR=3, CRC_ERROR=4, LENGTH_ERROR=5 — GFDIMessage.java:143-158.
MessageWriter — LE; writeString(s) = uint8 len + UTF-8, len ≤ 255 — messages/MessageWriter.java:55-63.

---

## 1. Protobuf-слой (Smart RPC)

### 1.1 Обёртка PROTOBUF_REQUEST/RESPONSE (5043/5044)

messages/ProtobufMessage.java:44-52, 82-93. Payload (LE):
```
uint16 requestId
uint32 dataOffset
uint32 totalProtobufLength
uint32 protobufDataLength
byte[protobufDataLength] chunk   // сериализованный Smart
```
* isChunked() = totalProtobufLength != protobufDataLength; isComplete() = dataOffset==0 && !isChunked().
* **maxChunkSize = 375** байт (проверено на Vívomove Style) — ProtocolBufferHandler.java:73.
* Исходящее >375 Б: первый чанк сразу, следующие — по приходу ProtobufStatus OK, срез [dataOffset+375, +375) — ProtocolBufferHandler.java:700-712.
* Входящие чанки собираются в chunkedFragmentsMap по requestId; dataOffset==0 перезаписывает — ProtocolBufferHandler.java:283-307.
* requestId исходящих: (last+1) % 65536 — ProtocolBufferHandler.java:97-100.

### 1.2 ProtobufStatusMessage

messages/status/ProtobufStatusMessage.java:31-45, 76-88.
Исходящий: `uint16 5000 | uint16 origMsgId | byte status | uint16 requestId | uint32 dataOffset | byte chunkStatus | byte statusCode`.
* ProtobufChunkStatus: KEPT=0, DISCARDED=1.
* ProtobufStatusCode: NO_ERROR=0, UNKNOWN_REQUEST_ID=100, DUPLICATE_PACKET=101, MISSING_PACKET=102, EXCEEDED_TOTAL_PROTOBUF_LENGTH=103, PROTOBUF_PARSE_ERROR=200, UNKNOWN=201.
* isOK = ACK && KEPT && NO_ERROR. Успешный статус также подтверждает чанк DataTransfer: DataTransferHandler.onDataChunkSuccessfullyReceived(requestId) — ProtocolBufferHandler.java:264.

### 1.3 Корневой message Smart (proto2, package garmin_vivomovehr)

gdi_smart_proto.proto:20-36:

| # | поле | тип |
|---|------|-----|
|1|calendar_service|CalendarService|
|2|http_service|**HttpService**|
|3|installed_apps_service|InstalledAppsService|
|4|app_config_service|AppConfigService|
|7|data_transfer_service|**DataTransferService**|
|8|device_status_service|DeviceStatusService|
|12|find_my_watch_service|**FindMyWatchService**|
|13|core_service|**CoreService**|
|16|sms_notification_service|SmsNotificationService|
|22|explore_sync_service|ExploreSyncService|
|27|authenticationService|AuthenticationService|
|39|ecg_service|EcgService|
|42|settings_service|SettingsService|
|43|file_sync_service|FileSyncService|
|49|notifications_service|**NotificationsService**|

Диспетчер — ProtocolBufferHandler.processIncoming, ProtocolBufferHandler.java:113-260: core → calendar → sms → http → dataTransfer → deviceStatus → findMyWatch → settings → authentication → notifications → installedApps → fileSync → ecg → appConfig → exploreSync. Нераспознано → ACK/DISCARDED/UNKNOWN_REQUEST_ID.

### 1.4 HttpService (gdi_http_service.proto)

```proto
message HttpService {
  enum Method { UNKNOWN_METHOD=0; GET=1; PUT=2; POST=3; DELETE=4; PATCH=5; HEAD=6; }
  enum Status { UNKNOWN_STATUS=0; OK=100; NETWORK_REQUEST_TIMED_OUT=200;
                FILE_TOO_LARGE=300; DATA_TRANSFER_ITEM_FAILURE=400; }
  enum ResponseType { JSON=0; URL_ENCODED=1; PLAIN_TEXT=2; XML=3; }
  enum Version { VERSION_1=0; VERSION_2=1; }

  optional WebRequest      webRequest     = 1;
  optional WebResponse     webResponse    = 2;
  optional RawRequest      rawRequest     = 5;
  optional RawResponse     rawResponse    = 6;
  optional ShowURLRequest  showURLRequest = 11;

  message WebRequest {                 // legacy, тело/заголовки в GarminJson
    required string url = 1;  optional Method method = 2;
    optional bytes headers = 3; optional bytes body = 4;
    optional uint32 maxResponseLength = 5;
    optional bool httpHeadersInResponse = 6 [default=true];
    optional bool compressResponseBody = 7 [default=false];
    optional ResponseType responseType = 8;
    optional Version version = 9 [default=VERSION_1];
  }
  message WebResponse { optional Status status=1; optional uint32 httpStatus=2; optional bytes body=3;
                        optional bytes headers=4; optional uint32 size=5; optional ResponseType responseType=6; }
  message RawRequest  { required string url=1; optional Method method=3; repeated Header header=5;
                        optional bool useDataXfer=6; optional bytes rawBody=7; }
  message RawResponse { optional Status status=1; optional uint32 httpStatus=2; optional bytes body=3;
                        optional DataTransferItem xferData=4; repeated Header header=5; }
  message DataTransferItem { required uint32 id=1; required uint32 size=2; }
  message Header { required string key=1; required string value=2; }
  message ShowURLRequest { required string url=1; optional bytes parameters=2; optional string app=3; }
}
```

### 1.5 DataTransferService (gdi_data_transfer_service.proto)

```proto
message DataTransferService {
  enum Status { UNKNOWN=0; SUCCESS=1; INVALID_ID=2; INVALID_OFFSET=3; }
  optional DataDownloadRequest  dataDownloadRequest  = 1;
  optional DataDownloadResponse dataDownloadResponse = 2;
  message DataDownloadRequest  { required uint32 id=1; required uint32 offset=2; optional uint32 maxChunkSize=3; }
  message DataDownloadResponse { required Status status=1; required uint32 id=2; required uint32 offset=3; optional bytes payload=4; }
}
```

### 1.6 FindMyWatchService (gdi_find_my_watch.proto)

```proto
message FindMyWatchService {
  optional FindMyWatchRequest        find_request    = 1;  // { required int32 timeout = 1; }
  optional FindMyWatchResponse       find_response   = 2;  // { optional ResponseStatus status = 1; }
  optional FindMyWatchCancelRequest  cancel_request  = 3;  // пустой
  optional FindMyWatchCancelResponse cancel_response = 4;
  enum ResponseStatus { UNKNOWN_RESPONSE_STATUS=0; OK=100; ERROR=200; }
}
```

### 1.7 NotificationsService — картинки уведомлений

```proto
message NotificationsService { optional PictureRequest pictureRequest=1; optional PictureResponse pictureResponse=2; }
message PictureRequest    { optional uint32 notification_id=1; optional uint32 unk2=2 /*0*/; optional PictureParameters parameters=3; }
message PictureParameters { optional uint32 width=1; optional uint32 height=2; optional uint32 unk3=3 /*204800 — макс размер?*/; optional uint32 quality=4 /*80*/; }
message PictureResponse   { optional uint32 unk1=1 /*1*/; optional uint32 notification_id=2; optional uint32 unk3=3 /*0*/;
                            optional uint32 unk4=4 /*1*/; optional DataTransferItem dataTransferItem=5; }
message DataTransferItem  { optional uint32 id=1; optional uint32 size=2; }
```

### 1.8 CoreService — локация/GPS/sync (gdi_core.proto)

```proto
message CoreService {
  optional SyncRequest sync_request=1;  optional SyncResponse sync_response=2;
  optional GetLocationRequest get_location_request=3;  optional GetLocationResponse get_location_response=4;
  optional LocationUpdatedSetEnabledRequest  location_updated_set_enabled_request=5;
  optional LocationUpdatedSetEnabledResponse location_updated_set_enabled_response=6;
  optional LocationUpdatedNotification location_updated_notification=7;

  enum DataType { SIGNIFICANT_LOCATION=0; GENERAL_LOCATION=1; REALTIME_TRACKING=2; INREACH_TRACKING=3; TRACKING_EVENT=4; }
  message GetLocationRequest  { optional RequestType request_type=1; enum RequestType { STANDARD=0; EMERGENCY=1; } }
  message GetLocationResponse { optional Status status=1; optional LocationData location_data=2;
     enum Status { OK=1; NO_VALID_LOCATION=2; LOCATION_SERVICES_UNAVAILABLE=3; LOCATION_SERVICES_DISABLED=4; TRY_AGAIN_LATER=5; } }
  message LocationUpdatedSetEnabledRequest { optional bool enabled=1; repeated Request requests=2; }
  message Request { optional DataType requested=1; optional float min_update_threshold=2; optional float distance_threshold=3; }
  message LocationUpdatedSetEnabledResponse { optional Status status=1; repeated Requested requests=2;
     enum Status { OK=1; UNAVAILABLE=2; UNKNOWN3=3; UNKNOWN4=4; }
     message Requested { optional DataType requested=1; optional RequestedStatus status=2; enum RequestedStatus { OK=1; KO=2; } } }
  message LocationUpdatedNotification { repeated LocationData location_data=1; }
  message LocationData { required LatLon position=1; required float altitude=2; required uint32 timestamp=3;
                         required float h_accuracy=4; required float v_accuracy=5; required DataType position_type=6;
                         required float bearing=9; required float speed=10; }
  message LatLon { required sint32 lat=1; required sint32 lon=2; }   // semicircles, zigzag
}
```
Обработка — ProtocolBufferHandler.java:414-490:
* GetLocationRequest → GENERAL_LOCATION; при lat==0 && lon==0 → NO_VALID_LOCATION.
* LocationUpdatedSetEnabledRequest: OK только для REALTIME_TRACKING и только при pref PREF_WORKOUT_SEND_GPS_TO_BAND, остальное KO; при OK стартует GPS-сервис с интервалом 1000 мс (ProtocolBufferHandler.java:466-479).
* Отправка позиции: onSetGpsLocation → Smart.core_service.location_updated_notification с REALTIME_TRACKING — GarminSupport.java:1430-1440.

### 1.9 DeviceStatusService

remote_device_battery_status_changed_notification=1, ..._request=2, ..._response=3 {status, int32 current_battery_level}, activity_status_request=4, activity_status_response=5 {ActivityStatus: OFF=0,STOPPED=1,PAUSED=2,ON=3}; ResponseStatus{UNKNOWN=0, OK=1, NO_REMOTE_DEVICE=2}. Battery → GBDeviceEventBatteryInfo — ProtocolBufferHandler.java:399-413.

### 1.10 SmsNotificationService

```proto
message SmsNotificationService {
  optional SmsSendMessageRequest sms_send_message_request=1;   // { receiver_number=1, message=2 }
  optional SmsSendMessageResponse sms_send_message_response=2;
  optional SmsCannedListChangedNotification sms_canned_list_changed_notification=3; // repeated CannedListType changed_type=1
  optional SmsCannedListRequest  sms_canned_list_request=4;    // repeated CannedListType requested_types=1
  optional SmsCannedListResponse sms_canned_list_response=5;   // { status=1, repeated SmsCannedList lists=2 }
  message SmsCannedList { optional CannedListType type=1; repeated string response=2; }
  enum CannedListType { PHONE_CALL_RESPONSE=0; SMS_MESSAGE_RESPONSE=1; }
  enum ResponseStatus { SUCCESS=0; GENERIC_ERROR=1; }
}
```
Реализация — ProtocolBufferHandler.java:539-600; шаблоны из prefs `canned_reply_1..16` (SMS) и `canned_message_dismisscall_1..16` (звонок).

### 1.11 SettingsService

definitionRequest=1/definitionResponse=2, stateRequest=3/stateResponse=4, changeRequest=5/changeResponse=6, initRequest=8/initResponse=9.
InitRequest{language="en_US", region="us"} шлётся при подключении, если есть capability REALTIME_SETTINGS — GarminSupport.java:452-467.
Модель: ScreenDefinition{screenId, unk3=928002, title:Label, repeated ScreenEntry}, ScreenEntry{id,type,title,icon,target,sortOptions,textOption}, Target.type: 0 subscreen / 1 list preference / 6 activity / 7 hidden / 9 subscreen с опциями, TargetNumberPicker{min,max,step}, ScreenState{screenId, repeated EntryState{id,state,switch,summary}}, Summary{valueList|valueTime|valueNumber|valueDate|valueHeight}, ChangeRequest{screenId, entryId, switch|option|time|number|position|newDate|text|height}, ResponseStatus{SUCCESS=0, GENERIC_ERROR=1}.

### 1.12 AuthenticationService

AuthenticationService{oauthRequest=1, oauthResponse=2}; OAuthResponse{keys=1, unk2=2}; OAuthKeys{consumerKey=1, consumerSecret=2, oauthToken=3, oauthSecret=4}. Фейк: consumerKey/oauthToken = случайный UUID, secrets = случайные 35 alnum, unk2=0 — ProtocolBufferHandler.java:161-186 (за pref fakeOauthEnabled).

### 1.13 Прочие сервисы

* **gdi_calendar_service.proto** — CalendarServiceRequest{begin,end,include_organizer/title/location/description/start_date/end_date/all_day, max_*_length, max_events}, CalendarEvent{organizer,title,location,description,start_date,end_date,all_day,reminder_time_in_secs}. Отдаём не более max_events*2; для all-day конверсия UTC→local — ProtocolBufferHandler.java:309-397.
* **gdi_installed_apps_service.proto** — AppType{UNKNOWN=0,WATCH_APP=1,WIDGET=2,WATCH_FACE=3,DATA_FIELD=4,ALL=5,NONE=6,AUDIO_CONTENT_PROVIDER=7,ACTIVITY=8}; InstalledApp{storeAppId(bytes),type,name,disabled,version,fileName,fileSize,nativeAppId,favorite}; GetInstalledAppsResponse{availableSpace,availableSlots,installedApps}; DeleteAppResponse.Status{UNKNOWN=0,OK=1,FAILED_TO_DELETE=2}.
* **gdi_app_config_service.proto** — appConfigGet=1/Status=2, appConfigSet=3/Status=4, appConfigRet=5; AppConfig{appId(bytes), appConfig(bytes)}; status: 1 = есть настройки/успех, 2 = не установлено/нет настроек.
* **gdi_file_sync_service.proto** (pref new_sync_protocol) — FileRequest=1/FileResponse=2, FileListRequest=9/FileListResponse=10, NewFileNotification=12, FileSetFlags=15, FileUpdateNotification=17; File{id:FileId, type:FileType, size, page_id}, FileId{fixed64 id1, fixed64 id2}, FileType{name, code: 8 monitor / 9 sports}; флаг already-synced = 0x000000000000a5a5 (42405); FileResponse.status: 0 success, 3 failure.
* **gdi_ecg_service.proto**, **gdi_explore_sync.proto** — за флагами new_sync_protocol / garmin_exploresync.

---

## 2. HTTP-прокси: часы просят телефон скачать URL

### 2.1 Поток

1. Часы шлют Smart.http_service (rawRequest или webRequest) внутри PROTOBUF_REQUEST.
2. ProtocolBufferHandler → HttpHandler.handle(httpService, requestId) — http/HttpHandler.java:56-82.
3. GarminHttpRequest нормализует: метод (Method.name()), Uri.parse(url), заголовки (rawRequest — ключи **lowercase**; webRequest — из GarminJson) — http/GarminHttpRequest.java:27-52, 95-102.
4. Цепочка перехватчиков, первый по supports() — HttpHandler.java:44-52: `WeatherInterceptor → AgpsInterceptor → ImageServiceInterceptor → ContactsInterceptor → OauthInterceptor → FirewallInterceptor (всегда последний)`. Интерфейс: http/interceptors/HttpInterceptor.kt:7-10 — supports(request): Boolean, handle(request): GarminHttpResponse?.
5. GarminHttpResponse{complete=true, status=200, headers LinkedHashMap, body []byte, onDataSuccessfullySentListener} — http/GarminHttpResponse.java. complete=false → ответ асинхронно через ProtobufResponseEvent(smart, messageRequestId).
6. Ответ: createRawResponse / createWebResponse; null от перехватчика → RawResponse{status=UNKNOWN_STATUS} (для web ещё httpStatus=0).

### 2.2 RawResponse: gzip и data_xfer

HttpHandler.createRawResponse — HttpHandler.java:126-190:
* rawRequest.useDataXfer == true: тело **не** кладётся в ответ; DataTransferHandler.registerData(body) → id; в ответе xferData{id, size=len(body)}; дальше часы вытягивают чанки через DataDownloadRequest{id, offset, maxChunkSize}.
* Иначе: если запросный заголовок `accept-encoding == "gzip"` (строгое сравнение полной строки!) — тело жмётся GZIP и добавляется заголовок `Content-Encoding: gzip`.
* Всегда: status=OK(100), httpStatus = HTTP-код, заголовки → repeated Header{key,value}.

### 2.3 DataTransfer (чанкинг больших тел)

http/DataTransferHandler.java:
* id — глобальный AtomicInteger со случайным стартом в [0, Integer.MAX_VALUE/2) (строка 23).
* handleDataDownloadRequest: maxChunkSize по умолчанию Integer.MAX_VALUE; чанк = data[offset : min(offset+maxChunkSize, len)]; неизвестный id → INVALID_ID, offset вне диапазона → INVALID_OFFSET (45-79).
* Подтверждение чанка = ProtobufStatus OK с тем же requestId → onDataChunkSuccessfullyReceived (87-115). Когда покрыт весь [0, len) без дыр (TreeMap чанков) — вызываются listeners, данные освобождаются.
* Данные держатся целиком в RAM (в Go лучше стримить).

### 2.4 WebResponse (legacy, GarminJson)

HttpHandler.createWebResponse — HttpHandler.java:262-346:
* Заголовки ответа кодируются GarminJson в поле headers, если httpHeadersInResponse (default true).
* **Лимит размера:** len(body) > webRequest.maxResponseLength → Status.FILE_TOO_LARGE(300), httpStatus=0 (сжатие не реализовано, TODO в коде).
* Если content-type == application/json — тело парсится как JSON и перекодируется в GarminJson; если responseType==JSON, а тело не json → DATA_TRANSFER_ITEM_FAILURE(400); иначе тело заворачивается как JSON-строка.
* size = 0 (маркер «не сжато»).

### 2.5 GarminJson — бинарный формат (http/GarminJson.java)

Big-endian, две секции:
```
[ AB CD AB CD | uint32 stringSectionLen | { uint16 len(вкл. \0); bytes; 00 }* ]   // секция опциональна
  DA 7A DA 7A | uint32 dataSectionLen   | <значения в порядке BFS>
```
Типы: NULL=0x00, SINT32=0x01, FLOAT=0x02, STRING=0x03 (uint32 offset в строковой секции), ARRAY=0x05 (uint32 count + элементы в очередь), BOOL=0x09 (+1 байт), MAP=0x0b (uint32 count + пары key,value в очередь), SINT64=0x0e, DOUBLE=0x0f — GarminJson.java:28-36.
Кодирование значений — **breadth-first** через очередь (encodeValueBreadthFirst, 81-160); декодирование симметрично через placeholder-ы (163-250). Строки дедуплицируются (LinkedHashSet), offset = позиция записи внутри строковой секции.

### 2.6 Перехватчики: домены и контракты

| Перехватчик | Домен + путь | Что делает |
|---|---|---|
| Weather | `api.gcs.garmin.com` **или** `cache.dciwx.com`, путь `/weather/…` | JSON-погода (§4.3) — WeatherInterceptor.java:49-52 |
| Agps | `api.gcs.garmin.com`, путь `/ephemeris/…` | локальный AGPS-файл (§3) — AgpsInterceptor.java:42-46 |
| ImageService | `api.gcs.garmin.com`, путь `/image-service/…` | иконки приложений (§5.6) — ImageServiceInterceptor.java:45-48 |
| Contacts | `connectapi.garmin.com`, путь `/device-gateway/usercontact/…` | protobuf-контакты — ContactsInterceptor.java:35-38 |
| Oauth | путь `/api/oauth/…`, `/oauth/…`, `/oauthTokenExchangeService/…` (любой домен) | фейковые токены — OauthInterceptor.java:50-54 |
| Firewall | всё остальное | проксирование наружу либо блок |

**Contacts** (ContactsInterceptor.java:41-113): только `/device-gateway/usercontact/contacts` и только при `accept: application/octet-stream`. Ответ — GarminContacts.Response{repeated Contact contact=1, Contact self=2}, Content-Type application/octet-stream.
```proto
message Contact { string id=1 /*32 hex uppercase, у себя "SELF"*/; string fullName=2; string firstName=3; string lastName=4;
                  repeated Phone phone=5; string unk7=7 /*""*/; uint32 unk8=8 /*0*/; uint32 unk9=9 /*0*/; uint32 unk10=10 /*0*/;
                  uint64 updateTime=11 /*ms*/; uint32 unk12=12 /*0*/; uint32 unk21=21 /*0*/; }
message Phone   { string number=1; uint32 unk2=2 /*1*/; uint32 unk3=3 /*1*/; uint32 unk4=4 /*0*/;
                  uint32 unk5=5 /*random 65535..131070*/; uint32 unk6=6 /*0*/; uint32 unk7=7 /*0*/; string unk8=8 /*""*/;
                  uint32 unk9=9 /*1*/; string id=10 /*UUID*/; uint32 unk11=11 /*1*/; repeated Unk12 unk12=12 /*{1,5},{2,5}*/; }
message Unk12   { uint32 unk1=1; uint32 unk2=2 /*5*/; }
```
Побочный эффект: pref `feat_contacts=true`.

**Oauth** (OauthInterceptor.java:57-180), требует POST:
* `/oauthTokenExchangeService/connectToIT` или `/oauth/connect_exchange/token` → JSON camelCase: `{accessToken:UUID, tokenType:"Bearer", refreshToken:UUID, expiresIn:7776000, scope:"<список>", refreshTokenExpiresIn:"31536000", customerId:UUID}`.
* `/api/oauth/token` или `/oauth/refresh_token/token` → JSON snake_case: `{access_token, token_type:"Bearer", expires_in:7776000, scope, refresh_token:<из тела grant_type=refresh_token&refresh_token=…&client_id=…>, refresh_token_expires_in:"31536000", customerId}`.
* Скоупы — фиксированный список (Swim 2 / Venu 3 / Enduro 3): GCS_EPHEMERIS_SONY_READ, GCS_CIQ_APPSTORE_MOBILE_READ, GCS_IMAGE_READ, GCS_IMAGE_STORAGE_READ, GCS_LIVETRACK_FIT_*, GCS_WEATHER_RACEDAY_READ, OMT_SUBSCRIPTION_READ и др. — строки 74-119.
* Если фейк-OAuth выключен — уведомление «auth expired» не чаще раза в неделю (604800000 мс).

**Firewall** (FirewallInterceptor.java:48-114):
* Блок, если нет интернета или запрос не прошёл InternetFirewall(WATCH_APP).
* **Жёсткий блок любых `*.garmin.com` и `*.dciwx.com`**, даже если пользователь их разрешил (фейковые OAuth-креды) — строки 61-68. ⇒ В Go-порте к Garmin ходить не надо: всё нужное отдают локальные перехватчики.
* Остальное уходит через AIDL IHttpService (InternetHelper); ответ асинхронный: сразу setComplete(false), потом HttpHandler.createSuccessResponse + ProtobufResponseEvent → prepareProtobufResponse (116-150).

**ShowURLRequest** (поле 11) не поддержан — только лог (HttpHandler.java:192-203).

---

## 3. AGPS (эфемериды)

* Часы запрашивают `https://api.gcs.garmin.com/ephemeris/...` через HTTP-прокси. **Приложение ничего само не качает** — файл берётся из локальной папки, выбранной пользователем (SAF).
* Prefs (devices/garmin/GarminPreferences.java:9-27):
  * `garmin_agps_known_urls` — список URL через `\n`, накапливается при каждом запросе (AgpsInterceptor.saveKnownUrl, 124-133);
  * `garmin_agps_folder` — URI папки;
  * `garmin_agps_filename_%s`, `garmin_agps_status_%s`, `garmin_agps_update_time_%s`, где `%s = md5(url)` (hex).
* Статусы: GarminAgpsStatus{CURRENT, PENDING, ERROR}. GarminCoordinator.supportsAgpsUpdates = known-urls непуст (GarminCoordinator.java:433-435).
* Отдача (AgpsInterceptor.handle, 48-122):
  * читается максимум **1 МБ** (FileUtils.readAll(in, 1024*1024), обычно ~60 КБ);
  * `etag = "\"" + md5(bytes) hex lowercase + "\""`; при совпадении с `if-none-match` → **304** с пустым телом;
  * иначе `cache-control: max-age=14400`, Content-Type = значение запросного `accept` (иначе `application/octet-stream`), 200 + тело;
  * после подтверждённой доставки (listener DataTransfer) статус → CURRENT, время → now (141-152).
* Валидация формата (agps/GarminAgpsFile.java):
  * query `constellations=GPS,GLONASS,...` → tar-архив (GBTarFile.isTarFile), внутри обязаны быть файлы по GarminAgpsDataType: GPS→`CPE_GPS.BIN`, GLONASS→`CPE_GLO.BIN`, GALILEO→`CPE_GAL.BIN`, QZSS→`CPE_QZSS.BIN` (agps/GarminAgpsDataType.java:3-5);
  * путь содержит `/rxnetworks/` → gzip (magic `1F 8B`), внутри первые 2 байта `01 00` (CPE_RXNETWORKS_HEADER), затем **big-endian uint32 timestamp**; отвергается, если в будущем или старше 604800 с (7 дней) — 51-88;
  * путь начинается с `/ephemeris/cpe/sony` → первые байты `2A 12 A0 02` (CPE_SONY_HEADER);
  * иначе — отказ («Refusing to send agps for unknown url»).
* Ошибка чтения/валидации → pref-статус ERROR.

---

## 4. Погода — два независимых канала

### 4.1 Источник данных: Open-Meteo (Pulse-специфика)

util/PulseWeather.java:
* pref `pulse_weather_source`: `auto` (Open-Meteo), `external` (Breezy и т.п.), `off`.
* Троттлинг **15 минут** (THROTTLE_MS, строка 51); HTTP-таймауты 10 с, `User-Agent: Pulse/1.0`.
* Запрос (84-95):
```
https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f
 &current=temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m,
          wind_direction_10m,surface_pressure,cloud_cover,dew_point_2m
 &hourly=temperature_2m,weather_code,relative_humidity_2m,wind_speed_10m,wind_direction_10m,
         precipitation_probability,uv_index,dew_point_2m,visibility
 &daily=weather_code,temperature_2m_max,temperature_2m_min,sunrise,sunset,uv_index_max,precipitation_probability_max
 &timezone=auto&timeformat=unixtime&wind_speed_unit=ms&forecast_days=7
```
* Температуры хранятся в **кельвинах** (toKelvin = round(C + 273.15)); WMO-коды → коды OpenWeatherMap (wmoToOwm, 296+); почасовой список — до 24 записей начиная с now-3600; visibility берётся из hourly (в current его нет); имя локации — Geocoder; ручной override через prefs `pulse_weather_auto/lat/lon`.
* После загрузки: Weather.setWeatherSpec(list) → GBApplication.deviceService().onSendWeather().

### 4.2 Канал A: FIT-погода (push через GFDI)

* Часы могут прислать **WEATHER_REQUEST (5014)**: payload `byte format | int32 latitude | int32 longitude | byte hoursOfForecast` (LE) — messages/WeatherMessage.java:19-26 → WeatherRequestDeviceEvent.
* GarminSupport.evaluateGBDeviceEvent: WeatherRequestDeviceEvent → sendWeatherConditions(Weather.getWeatherSpec()) — GarminSupport.java:443-447.
* Только если девайс поддерживает GarminCapability.WEATHER_CONDITIONS; иначе погода уйдёт по HTTP (§4.3) — GarminSupport.java:737-742.
* Состав FIT (encodeWeather — GarminSupport.java:757-853):
  * 1 запись WeatherReport.current: timestamp, observedAtTime, temperature, low/high, condition, windDirection, precipProb, windSpeed, feelsLike, humidity, observedLocationLat/Long, airQuality, dewPoint, location(строка);
  * до **12** записей hourly_forecast (hour 0..11), один локальный message type: timestamp, temp, condition, windDir, windSpeed, precipProb, feelsLike(=temp), humidity, dewPoint, uvIndex, airQuality(null — чтобы поле попало в definition), atmosphericPressure;
  * daily_forecast: «сегодня» + до **4** дней, общий локальный message type ⇒ **набор полей обязан совпадать**; ts дня = weather.timestamp + (day+1)*86400.
* Конверсии (fit/messages/FitWeather.java:36-170):
  * температура: **kelvin - 273** (намеренно «неправильно», issue #4313) для temp, feelsLike, high, low, dewPoint;
  * ветер: mm/s = round(kmh / 3.6 * 1000), клип на 0xFFFE;
  * uvIndex: клип 0..10 (в WeatherSpec 0..15);
  * humidity/precipProb: клип 0..100; windDirection: % 360; отрицательные значения → null;
  * давление: pascal = round(millibar * 100);
  * AQI → enum (FieldDefinitionWeatherAqi.aqiAbsoluteValueToEnum), condition ← OWM-код (FieldDefinitionWeatherCondition.openWeatherCodeToFitWeatherStatus);
  * dayOfWeek — из timestamp в системной таймзоне.
* Передача — через FitLocalMessageHandler (сначала definitions, затем данные, с учётом maxPacketSize).

### 4.3 Канал B: HTTP-погода (pull через прокси)

http/interceptors/WeatherInterceptor.java. Комментарий в коде: «These get requested on connection **at most every 5 minutes**» (строка 55). Ответ всегда 200 + `Content-Type: application/json` (Gson; null-поля не сериализуются).

| Путь | Параметры (дефолты) | Ответ |
|---|---|---|
| `/weather/v1/forecast/day`, `/weather/v2/forecast/day` | lat, lon, duration=5, tempUnit=CELSIUS; v2 ещё provider=dci, speedUnit=KILOMETERS_PER_HOUR | массив WeatherForecastDay |
| `/weather/v1/forecast/hour`, `/weather/v2/forecast/hour` | lat, lon, duration=13, speedUnit=METERS_PER_SECOND, tempUnit=CELSIUS; v1: pressureUnit=MILLIBAR; v2: provider=dci, timesOfInterest | массив WeatherForecastHour |
| `/weather/v1/current`, `/weather/v2/current` | lat, lon, tempUnit=CELSIUS, speedUnit=METERS_PER_SECOND; v2 provider=dci | объект WeatherForecastCurrent |
| `/weather/pointWinds` | lat, lon, rspFmt=json (иное — отказ) | `{CcPointWinds:{i,lat,lon,W:[{t,s,d,g}]}}` |
| (`/weather/v1/calibration/altimeter`) | закомментирован | `{temperature:{value,uncertainty}, pressure, forecastTime, forecastIssueTime}` |

Структуры (WeatherInterceptor.java:186-420):
* `WeatherForecastDay{dayOfWeek, description, summary, high:WeatherValue, low, precipProb, icon, epochSunrise, epochSunset, wind{speed:WeatherValue, directionString, direction}, humidity}`. **dayOfWeek: v2 — 1=понедельник (BLETypeConversions.dayOfWeekToRawBytes), v1 — 1=воскресенье (Calendar.DAY_OF_WEEK).** Восход/закат из прогноза, иначе считаются SPA-солвером по последней локации.
* `WeatherForecastHour{epochSeconds, description, temp, precipProb, wind, icon, dewPoint, uvIndex (v2 — 3 знака, v1 — 1), relativeHumidity, feelsLikeTemperature, visibility, pressure, airQuality (только v2), cloudCover}`.
* `WeatherForecastCurrent{epochSeconds, temperature, description, icon, feelsLikeTemperature, dewPoint, relativeHumidity, wind, locationName, visibility{value,"METER"}, pressure{hPa*0.02953,"INCHES_OF_MERCURY"}, pressureChange, cloudCoverage}`.
* `WeatherValue{value, units}`; температура: CELSIUS = K-273, FAHRENHEIT = celsiusToFahrenheit(K-273.15), KELVIN = K; скорость: METERS_PER_SECOND = kmh/3.6, KILOMETERS_PER_HOUR = kmh.
* pointWinds: t = смещение секунд от первого часа, s = узлы (kmh/1.852), d = градусы, g (порывы) = s*1.47; максимум 4 точки.
* directionString — 8 румбов N/NE/E/SE/S/SW/W/NW, индекс = round(deg/45) % 8.
* AQI → 0..5: <20 Good, <50 Moderate, <100 UnhealthySensitive, <150 Unhealthy, <250 VeryUnhealthy, иначе Hazardous; -1 → null.
* mapToGarminCondition(owmCode) (471-620): гроза → 27; шторм/ветер/торнадо → 46; дождь/морось → 17; ледяной дождь и дождь со снегом → 40; снег → 38; туман/дымка/пыль → 47; ясно (800, 904) → 5; 801/802 → 8; 803/804 → 15; по умолчанию → 35. В комментариях есть карта иконок Venu 3 (0..51).

### 4.4 Настройки погоды на девайсе

sendDeviceSettings() при инициализации шлёт **DEVICE_SETTINGS (5026)**: AUTO_UPLOAD_ENABLED=true, WEATHER_CONDITIONS_ENABLED=true, WEATHER_ALERTS_ENABLED=false — GarminSupport.java:1059-1065.
Формат (messages/SetDeviceSettingsMessage.java:19-40):
```
uint16 5026 | byte count | { byte settingOrdinal, <value> }*
value: String  → uint8 len + UTF-8
       Integer → byte 4 + int32 (LE)
       Boolean → byte 1 + byte(0|1)
```
GarminDeviceSetting ordinals: 0 DEVICE_NAME, 1 CURRENT_TIME, 2 DAYLIGHT_SAVINGS_TIME_OFFSET, 3 TIME_ZONE_OFFSET, 4 NEXT_DAYLIGHT_SAVINGS_START, 5 NEXT_DAYLIGHT_SAVINGS_END, 6 AUTO_UPLOAD_ENABLED, 7 WEATHER_CONDITIONS_ENABLED, 8 WEATHER_ALERTS_ENABLED.

---

## 5. Уведомления

Схема ANCS-подобная: телефон анонсирует уведомление (**5033**), часы запрашивают атрибуты (**5034**), телефон шлёт данные чанками (**5035**), часы подтверждают (5000 + NotificationDataStatus). Подписка — **5036**.

### 5.1 NOTIFICATION_SUBSCRIPTION (5036)

Входящее: `byte enable(1=on) [ + byte unk ]` — messages/NotificationSubscriptionMessage.java:24-29.
Ответ (status/NotificationSubscriptionStatusMessage.java:22-33): `uint16 5000 | uint16 5036 | byte Status | byte NotificationStatus(ENABLED=0, DISABLED=1) | byte enableRaw | byte unk`.
Фактический статус — из pref PREF_SEND_APP_NOTIFICATIONS (GarminSupport.java:475-495); notificationsHandler.setEnabled(enable); при disabled хендлер игнорирует всё.

### 5.2 NOTIFICATION_UPDATE (5033) — анонс

messages/NotificationUpdateMessage.java:31-42:
```
uint16 5033 | byte updateType | byte categoryFlags | byte categoryValue | byte count | int32 notificationId | byte phoneFlags
```
* NotificationUpdateType: ADD=0, MODIFY=1, REMOVE=2.
* categoryFlags — битовый вектор NotificationFlag{BACKGROUND=bit0, FOREGROUND=bit1, UNK=bit2, ACTION_ACCEPT=bit3, ACTION_DECLINE=bit4} (1<<ordinal). Всегда добавляется ACTION_DECLINE; FOREGROUND — для всех известных типов (phone/email/sms/chat/navigation/social/alarm/generic).
* categoryValue — ordinal NotificationCategory{OTHER=0, INCOMING_CALL=1, MISSED_CALL=2, VOICEMAIL=3, SOCIAL=4, SCHEDULE=5, EMAIL=6, NEWS=7, HEALTH_AND_FITNESS=8, BUSINESS_AND_FINANCE=9, LOCATION=10, ENTERTAINMENT=11, SMS=12}. Маппинг: generic_phone→INCOMING_CALL, generic_email→EMAIL, generic_sms|generic_chat→SMS, generic_navigation→LOCATION, generic_social→SOCIAL, остальное OTHER.
* count — сколько уведомлений этого типа сейчас в очереди.
* phoneFlags — вектор NotificationPhoneFlags{LEGACY_ACTIONS=bit0, NEW_ACTIONS=bit1, HAS_ATTACHMENTS=bit2}; useLegacyActions=false захардкожено.
* Очередь в NotificationsHandler: до **64** штук, при переполнении вытесняется старейшее; повтор id → MODIFY — NotificationsHandler.java:96-120.

### 5.3 NOTIFICATION_CONTROL (5034) — запросы с часов

messages/NotificationControlMessage.java:70-131. Первый байт — NotificationCommand: **GET_NOTIFICATION_ATTRIBUTES=0, GET_APP_ATTRIBUTES=1, PERFORM_LEGACY_NOTIFICATION_ACTION=2, PERFORM_NOTIFICATION_ACTION=128**.
* GET_NOTIFICATION_ATTRIBUTES: `int32 notificationId`, далее список `byte attributeId [+ uint16 maxLength]`. NotificationAttribute: APP_IDENTIFIER=0, TITLE=1(len), SUBTITLE=2(len), MESSAGE=3(len), MESSAGE_SIZE=4, DATE=5, NEGATIVE_ACTION_LABEL=7, ACTIONS=127 (uint16 + 1 доп. байт), ATTACHMENTS=128 — NotificationsHandler.java:355-380.
* PERFORM_LEGACY_NOTIFICATION_ACTION: `int32 id | byte action` (LegacyNotificationAction{ACCEPT=0, REFUSE=1}) — только логируется.
* PERFORM_NOTIFICATION_ACTION: `int32 id | byte actionCode [ | null-terminated UTF-8 строка ]` (в новых прошивках строки может не быть).
* GET_APP_ATTRIBUTES: `null-terminated appIdentifier` + список `byte attrId` (AppAttribute.APP_NAME=0).
* Ответ-статус — NotificationControlStatusMessage(ACK, chunkStatus OK, statusCode NO_ERROR).

### 5.4 NOTIFICATION_DATA (5035)

Полезная нагрузка (NotificationsHandler.getNotificationDataMessage, 246-265):
```
byte 0 (GET_NOTIFICATION_ATTRIBUTES)
int32 notificationId
{ byte attributeId, uint16 valueLen, bytes value }*      // MESSAGE_SIZE всегда идёт ПОСЛЕДНИМ
```
Значения (NotificationAttribute.getNotificationSpecAttribute, 400-437):
* DATE — формат `yyyyMMdd'T'HHmmss` (Locale.ROOT), when==0 → now;
* TITLE — для SMS: sender / phoneNumber / "-"; иначе title;
* SUBTITLE — subject; MESSAGE — body; MESSAGE_SIZE — десятичная длина body строкой;
* APP_IDENTIFIER — sourceAppId; ATTACHMENTS — строка "1" (число вложений);
* ACTIONS — см. §5.5;
* обрезка: при maxLength != 0 → substring(0, min(len, maxLength)) **по Java-символам**, затем UTF-8.

GET_APP_ATTRIBUTES-ответ (267-292): `byte 1 | appId UTF-8 | 0x00 | { byte attrCode, uint16 len, bytes appName }`.

Формат GFDI-пакета 5035 (messages/NotificationDataMessage.java:29-38):
```
uint16 5035 | uint16 messageSize (полный) | uint16 crc (running) | uint16 dataOffset | bytes chunk
```
Чанкинг (NotificationsHandler.NotificationFragment, 550-590): **maxBlockSize = 300** байт; runningCrc инкрементально: crc = ChecksumCalculator.computeCrc(prevCrc, chunk, 0, len).
Подтверждение — NotificationDataStatusMessage: `byte status | byte transferStatus`, TransferStatus{OK=0, RESEND=1, ABORT=2, CRC_MISMATCH=3, OFFSET_MISMATCH=4} (status/NotificationDataStatusMessage.java:36-50). При OK шлётся следующий чанк; когда буфер исчерпан — финальный NotificationDataStatusMessage(NOTIFICATION_DATA, ACK, OK) (NotificationsHandler.java:528-548).

### 5.5 Действия (ACTIONS) — кодировка

NotificationsHandler.encodeNotificationActionsString / encodeNotificationAction, 439-500:
```
byte actionCount
{ byte actionCode, byte iconPositionBitVector, byte descLen, bytes desc(UTF-8) }*
```
Если действий нет — 4 нулевых байта `00 00 00 00`.
NotificationAction коды: CUSTOM_ACTION_1..5 = 1..5, **REPLY_INCOMING_CALL=94** (icon BOTTOM), **REPLY_MESSAGES=95** (BOTTOM), **ACCEPT_INCOMING_CALL=96** (RIGHT), **REJECT_INCOMING_CALL=97** (LEFT), **DISMISS_NOTIFICATION=98** (LEFT), **BLOCK_APPLICATION=99** (без иконки → 0x00) — 502-530.
NotificationActionIconPosition{BOTTOM=bit0, RIGHT=bit1, LEFT=bit2} (1<<ordinal).
Для GENERIC_PHONE всегда добавляются 3 действия с описанием " " (текст на часах не показывается): REPLY_INCOMING_CALL, REJECT_INCOMING_CALL, ACCEPT_INCOMING_CALL. Кастомных simple-действий максимум **5**.

Обратная обработка (performNotificationAction, 189-244):
* REPLY_INCOMING_CALL → CallControl.REJECT **и** (fall-through) REPLY;
* REPLY_MESSAGES → NotificationControl.REPLY с текстом из actionString; для phone/SMS передаётся phoneNumber, иначе handle wearable-действия из LimitedQueue mNotificationReplyAction (32 записи);
* ACCEPT_INCOMING_CALL → CallControl.ACCEPT; REJECT_INCOMING_CALL → REJECT;
* DISMISS_NOTIFICATION → DISMISS; BLOCK_APPLICATION → MUTE;
* CUSTOM_ACTION_1..5 → REPLY с handle по порядковому индексу среди TYPE_WEARABLE_SIMPLE/TYPE_CUSTOM_SIMPLE.

### 5.6 Картинки и иконки

* **Вложение уведомления (фото)**: часы шлют Smart.notifications_service.pictureRequest{notification_id, parameters{width, height, unk3=204800, quality=80}}. Телефон масштабирует по ширине с сохранением пропорций, жмёт JPEG с quality, регистрирует в DataTransfer и отвечает pictureResponse{unk1=1, notification_id, unk3=0, unk4=1, dataTransferItem{id,size}} — ProtocolBufferHandler.java:492-537.
* **Иконка приложения** — по HTTP: `GET https://api.gcs.garmin.com/image-service/v2/device/images/details?ownerAliasId=<package>&imageSize=<N>` (ImageServiceInterceptor.java:52-101), только GET.
  Ответ: `Content-Type: image/png`, заголовки `imagetype: ICON`, `original-image-size: <len(png)>`, `ownerid: <package>`, `ownertype: APP`; тело = **43-байтный заголовок + PNG** (иконка рендерится в ARGB_8888 NxN, PNG quality 100).
  Заголовок (BE, createImageHeader, 134-177):
  `0C 11 EE 5E | uint32(pngLen + 0x23) | uint16 size | uint16 size | FF FF 00 40 00 00 10 00 00 1C 00 00 1C 10 | uint16 size | uint16 size | 00 02 04 00 00 00 00 04 00 | uint32 pngLen (LE, BLETypeConversions.fromUint32)`.
  Нет иконки → **404** + JSON `{requestId:UUID, errors:[{message:"Owner alias (<id>) not found", type:"NOT_FOUND"}]}` и заголовок `x-request-id`.

---

## 6. Звонки

Отдельного call-сообщения нет — звонок это уведомление типа GENERIC_PHONE (NotificationsHandler.onSetCallState, 72-95):
* `id = firstNonBlank(callSpec.number, "Gadgetbridge Call").hashCode()`;
* CALL_INCOMING → NotificationSpec{phoneNumber, sourceAppId, title=body=caller (name→number→"unknown"), type=GENERIC_PHONE} + **пустой фиктивный Action** (взводит флаг hasActions; реальные действия на часах захардкожены) → NotificationUpdateMessage(ADD, …);
* любое другое состояние → onDeleteNotification(id) → NotificationUpdateMessage(REMOVE, …);
* категория → INCOMING_CALL(1), флаг FOREGROUND;
* управление с часов: коды 94/96/97 (§5.5) → GBDeviceEventCallControl{ACCEPT|REJECT}; ответ текстом — 94 + строка;
* шаблоны ответов на звонок — SmsNotificationService.CannedListType.PHONE_CALL_RESPONSE (§1.10).

---

## 7. Find my phone / find my watch

* **Часы ищут телефон**: **FIND_MY_PHONE_REQUEST (5039)** — payload `byte duration` → GBDeviceEventFindPhone.START, ответ обычный ACK (messages/FindMyPhoneRequestMessage.java:20-33); **FIND_MY_PHONE_CANCEL (5040)** — пустой payload → GBDeviceEventFindPhone.STOP (messages/FindMyPhoneCancelMessage.java).
* **Телефон ищет часы** (GarminSupport.onFindDevice, 1073-1088): Smart.find_my_watch_service.find_request{timeout: 60} при старте, cancel_request{} при остановке — как PROTOBUF_REQUEST. Ответы часов только логируются — ProtocolBufferHandler.java:602-611.

---

## 8. Музыка (GFDI, без protobuf)

* **MUSIC_CONTROL_CAPABILITIES (5042)** — часы спрашивают возможности; payload либо пустой, либо 1 байт. Ответ вместо статуса (messages/MusicControlCapabilitiesMessage.java:24-36): `uint16 5000 | uint16 5042 | byte ACK(0) | byte commandCount | byte[commandCount] ordinals`. GarminMusicControlCommand (ordinal = код): TOGGLE_PLAY_PAUSE=0, SKIP_TO_NEXT_ITEM=1, SKIP_TO_PREVIOUS_ITEM=2, VOLUME_UP=3, VOLUME_DOWN=4, PLAY=5, PAUSE=6, SKIP_FORWARD=7, SKIP_BACKWARDS=8. Анонсируются все 9.
* **MUSIC_CONTROL (5041)** — входящее, `byte commandOrdinal` → GBDeviceEventMusicControl (реально обрабатываются первые 5) — messages/MusicControlMessage.java:16-42.
* **MUSIC_CONTROL_ENTITY_UPDATE (5049)** — исходящее (messages/MusicControlEntityUpdateMessage.java:49-70):
```
uint16 5049 | { byte (valueLen+3), byte entityId, byte attrOrdinal, byte 0x00, bytes value(UTF-8) }*
```
  Значение ≤ 252 байт. Сущности: PLAYER(entityId=0){NAME=0, PLAYBACK_INFO=1, VOLUME=2}, QUEUE(1){INDEX=0, COUNT=1, SHUFFLE=2, REPEAT=3}, TRACK(2){ARTIST=0, ALBUM=1, TITLE=2, DURATION=3}.
  Использование (GarminSupport.java:1305-1364): onSetMusicInfo → TRACK.ARTIST/ALBUM/TITLE/DURATION (duration — секунды строкой); onSetMusicState → PLAYER.PLAYBACK_INFO = String.format(Locale.ROOT, "%d,%.1f,%.3f", playing, playRate, progress); onSetPhoneVolume → PLAYER.VOLUME = "%.2f" от volume/100f.

---

## 9. Device events (мост «протокол → GarminSupport»)

GARMIN/deviceevents/:
* WeatherRequestDeviceEvent(format, latitude, longitude, hoursOfForecast) → триггер FIT-погоды;
* NotificationSubscriptionDeviceEvent{boolean enable} → вкл/выкл NotificationsHandler + ответ 5036;
* ProtobufResponseEvent{Smart payload, int messageId} → асинхронный ответ на PROTOBUF_REQUEST;
* CapabilitiesDeviceEvent{Set<GarminCapability>} → completeInitialization() + init realtime settings;
* MaxPacketSizeDeviceEvent{int maxPacketSize} → размер пакета для FIT/файлов;
* SupportedFileTypesDeviceEvent{List<FileType>}, FileDownloadedDeviceEvent{success, directoryEntry, localPath}, IncomingFitDefinitionDeviceEvent{List<RecordDefinition>} — файловый слой.
Все evaluate() пустые: обработка в GarminSupport.evaluateGBDeviceEvent — GarminSupport.java:441-556.

Инициализация после CapabilitiesDeviceEvent (GarminSupport.completeInitialization, 855-881): SUPPORTED_FILE_TYPES_REQUEST → sendDeviceSettings() → onSetTime (если разрешено) → SystemEvent SYNC_READY → enableBatteryLevelUpdate → state INITIALIZED; при первом коннекте ещё PAIR_COMPLETE, SYNC_COMPLETE, SETUP_WIZARD_COMPLETE.

---

## 10. Заметки для Go-порта

1. **Весь GFDI — little-endian**; protobuf внутри — обычный proto2 wire; GarminJson (BE, две секции) нужен только для legacy WebRequest/WebResponse.
2. Три разных чанкинга: protobuf **375 Б**, notification data **300 Б**, data-transfer — по maxChunkSize из запроса часов.
3. Подтверждение доставки data-transfer приходит как ProtobufStatus по requestId — без него не сработает listener AGPS «CURRENT».
4. Наружу реально нужен только api.open-meteo.com; все *.garmin.com / *.dciwx.com обслуживаются локально (weather / agps / image-service / contacts / oauth), а FirewallInterceptor их принудительно блокирует.
5. Температуры сдвигаются как K-273 (не 273.15) — намеренно; повторить дословно.
6. Обрезка атрибутов уведомлений идёт по Java-символам (UTF-16 code units) до UTF-8-кодирования — в Go резать по рунам и проверять итоговую длину.
7. MessageWriter.writeString — 1 байт длины, максимум 255 байт; music entity value ≤ 252 байт.
8. Ответ на 5042 (music capabilities) и на 5036 (notification subscription) — не generic ACK, а расширенные RESPONSE-пакеты; generic ACK там сломает часы.
