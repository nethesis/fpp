# FPP (what the Fuck Push Proxy)

This server is designed to be used as push notification proxy for [Android NethCTI app](https://github.com/nethesis/nethcti-app-android).
and [iOS NethCTI app](https://github.com/nethesis/nethcti-app-iphone).

The proxy handles:

- devices registration and deregistration
- background push notifications to [Google Firebase Cloud Messaging](https://firebase.google.com/docs/cloud-messaging) for Android phones
- VoIP push notifications to [Apple APN](https://developer.apple.com/documentation/usernotifications) for iOS devices

Call notifications are sent from [NethVoice VoIP PBX](https://github.com/nethesis/nethserver-nethvoice/).
The PBX server must have a valid entitlement to access the proxy.

Before starting the server, make sure to get:
- a valid [Firebase service account in JSON format](https://firebase.google.com/docs/admin/setup)
- a valid Apple APN p8 credentials file

## Usage

First download a Firebase service account and save it to a file named `credentials.json` and 
an Apple service account and save it to a file named `credentials.p8`.
Then execute the server:
```
GOOGLE_APPLICATION_CREDENTIALS="./credentials.json" APPLE_APPLICATION_CREDENTIALS="./credentials.p8" ./fpp
```

The server exposes the following APIs:

- `/metrics`: GET, return metrics in Prometheus format

- `/register`: POST, register a device. Both the device token and the topic must be a string of 64 hex characters.
  If device token and topic are valid, the server will save the token associated to the given topic.
  Token/topic association will expire after 90 days: applications must be opened at least once every 3 months
  to keep receiving notifications.
  This endpoint must be validated with an header `Instance-Token`, see `INSTANCE_TOKEN` env var.
  Parameters:
  - `token`: Apple Device token
  - `topic`: Unique topic to identify the mobile device. The topic is the sha256sum of the token received by the client
  - `type`: Registration type, can be `apple` for iOS devices and `firebase` for Android devices

- `/deregister`: POST, deregister a device. Both the device token and the topic must be a string of 64 hex characters.
  If the tuple token/topic is valid, the tuple will be deleted from the database.
  This endpoint must be validated with an header `Instance-Token`, see `INSTANCE_TOKEN` env var.
  Parameters:
  - `token`: Apple Device token
  - `topic`: Unique topic to identify the mobile device. The topic is the sha256sum of the token received by the client
  - `type`: Registration type, can be `apple` for iOS devices and `firebase` for Android devices

- `/send`: POST, send a wake-up notification to a device
  Parameters:
  - `type`: Notification type, can be `apple` or `firebase`.
    The device must be already registered: the server will wake up the device token
    corresponding to the given `topic`
  - `topic`: Unique topic to identify the mobile device. The topic is the sha256sum of the token received by the client
    after the login
  - `uuid`: Flexisip transaction identifier
  - `call-id`: Asterisk call identifier
  - `from-uri`: Caller SIP URI
  - `display-name`: Caller display name

The server can be configured using the following environment variables:
- `GOOGLE_APPLICATION_CREDENTIALS`: path of the Firebase service account JSON file
- `APPLE_APPLICATION_CREDENTIALS`: path to the APN account p8 file
- `LISTEN`: (optional) listen address and port, default is `127.0.0.1:8080`
- `APPLE_TEAM_ID`: Apple Team ID for p8 credentials
- `APPLE_KEY_ID`: Apple Key ID for p8 credentials
- `APPLE_TOPIC`: topic for Apple APN, like `it.nethesis.nethcti3.voip` (note the `.voip` suffix)
- `APPLE_ENVIRONMENT` can be `production` or `sandbox`
- `DB_PATH`: path for device [database](#database)
- `INSTANCE_TOKEN`: a SHA25sum hash string representing a token for this instance.
   Requests to `/register` and `/deregister` token are validated against this token.
   This token must be compiled inside each mobile app

Send a notification using curl, example:
```
curl -H "Accept: application/json" http://localhost:8080/send \
  --data '{"type": "firebase", "topic": "b62aabfc0699e752fdbfda027433342f7bd20715da07049956d0daf20e34f326", "uuid": "550e8400-e29b-41d4-a716-446655440000", "call-id": "9b5ce40a-b167-41b4-b245-afbc51fc74ec&to=sip%3A91402", "display-name": "Test user", "from-uri": "sip:401@127.0.0.1"}'
```

Register an iOS device, example:
```
curl -H "Accept: application/json" -H "Instance-Token: 8dc657c146dc410790adb71d0be62c591b9bfda9c1c889972c0d2a095b71f733" "http://localhost:8080/register \
  --data '{"token": "A6F31E46858BF045475A529F709D781B8D48507DFFDCEBBB1DCEF907FD58AC05", "topic": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "type": "apple"}'
```

## Logging

Each request is logged to standard error in CSV format to ease future data analysis.
The standard error is redirect to syslog using systemd unit.

Each line can have 3 different formats.

### Send

Log of `send` requests has the following format:
```
datetime_rfc3339,action,type,result,response,topic,callid,uuid
```

Example of successful request:
```
2023-07-17T13:06:08Z,send,apple,success,9F070589-4EAA-52F9-2256-1097235DB00E,896dbae25cf505e8d051e4cb23549c1e34954310cb3aeffee6222715256b31a4,9feef923-0e01-4ba2-bc8e-52fc25a49965,<urn:uuid:32919e39-bc2a-006e-863d-7528bf8d2b46>
```

On success, the `response` field contains the Firebase/APN notification id.
On error, the `response` field contains the specific error message.

### Register and Deregister

Log of `register` and `deregister` request has the following format:
```
datetime_rfc3339,action,type,result,response,token,topic
```

Example of successful request:
```
2023-07-17T14:18:07Z,register,apple,success,ok,BC3CBEBF386158AEF1E54D2E4F7721A1944A11E25AEF86D9F6F809716F15C6B1,8c557d2568606950414e08caf498f42df9c7a5e62b6fd26dc34f9e5dab8c00ad
```

On error, the `response` field contains the specific error message.

### Invalid request

If the POST data can't be decoded, like in case of an invalid JSON,  requests are loggend in this special format:
```
datetime_rfc3339,"invalid",endpoint,result,error
```

Example:
```
2023-07-24T14:48:09Z,invalid,register,error,unexpected EOF
```

## Metrics

The `/metrics` endpoint returns [basic proces information](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus/collectors#NewProcessCollector)
plus some specific FPP metrics. FPP metrics have the `fpp_`prefix.

Example:
```
# HELP fpp_apn_error_count Number of errored Apple APN notifications.
# TYPE fpp_apn_error_count counter
fpp_apn_error_count 0
# HELP fpp_apn_success_count Number of successfull Apple APN notifications.
# TYPE fpp_apn_success_count counter
fpp_apn_success_count 0
# HELP fpp_firebase_error_count Number of errored Google Firebase notifications.
# TYPE fpp_firebase_error_count counter
fpp_firebase_error_count 0
# HELP fpp_firebase_success_count Number of successfull Google Firebase notifications.
# TYPE fpp_firebase_success_count counter
fpp_firebase_success_count 0
# HELP fpp_registered_apn_devices Number of registered Apple APN devices.
# TYPE fpp_registered_apn_devices gauge
fpp_registered_apn_devices 0
# HELP fpp_registered_firebase_devices Number of registered Google Firebase devices.
# TYPE fpp_registered_firebase_devices gauge
fpp_registered_firebase_devices 0
# HELP fpp_total_send_count Number of sent notifications.
# TYPE fpp_total_send_count counter
fpp_total_send_count 0
# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 0.26
# HELP process_max_fds Maximum number of open file descriptors.
# TYPE process_max_fds gauge
process_max_fds 524288
# HELP process_open_fds Number of open file descriptors.
# TYPE process_open_fds gauge
process_open_fds 20
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
process_resident_memory_bytes 3.0277632e+07
# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1.68976139934e+09
# HELP process_virtual_memory_bytes Virtual memory size in bytes.
# TYPE process_virtual_memory_bytes gauge
process_virtual_memory_bytes 3.70067456e+09
# HELP process_virtual_memory_max_bytes Maximum amount of virtual memory available in bytes.
# TYPE process_virtual_memory_max_bytes gauge
process_virtual_memory_max_bytes 1.8446744073709552e+19
# HELP promhttp_metric_handler_errors_total Total number of internal errors encountered by the promhttp metric handler.
# TYPE promhttp_metric_handler_errors_total counter
promhttp_metric_handler_errors_total{cause="encoding"} 0
promhttp_metric_handler_errors_total{cause="gathering"} 0
```

A [Grafana](https://grafana.com/) dashboard is available inside the `deploy` directory.
Just import the file named `grafana_dashboard.json".

## Database

Registered devices are kept inside a local persistent [Badger](https://github.com/dgraph-io/badger) database.
The device token/topic association is a key-value entry in the form: `<topic>:<device_token>`.
Each entry has a TTL of 3 months which is extended on every `/send`.

If the FPP instance is not running, the database can be inspected with a command line tool:
```
curl -sL https://github.com/dgraph-io/badger/releases/download/v4.1.0/badger-linux-amd64.tar.gz | tar xvz
mv badger-linux-amd64 /usr/local/bin/badger
cd /var/local/fpp/nethesis/db
badger info --read-only --dir . --show-keys
```

## Build and deploy

Fpp can be deployed manually or using the [Ansible role](deploy/ansible/roles/fpp/README.md).

### Manual setup
The deploy procedure should:
- configure 2 FPP instances for every branding: one for production and one for sandbox;
  the sandbox environment is mandatory to test iOS applications running inside [Xcode](https://developer.apple.com/xcode/)
- configure a Traefik instance to authenticate the requests and forward them to the right FPP instance:
  - `send` endpoint must be authenticated by Traefik using `my.nethesis.it`
  - `register` and `deregister` endpoints should not be authenticated by Traefik


Build and deploy on Fedora server:
```
setenforce 0
dnf install git podman golang
git clone git@github.com:nethesis/fpp.git
cd fpp
go get && go build
cp fpp /usr/bin/fpp
cp deploy/fpp@.service /etc/systemd/system
systemctl daemon-reload

mkdir -p /var/local/fpp/nethesis
cp credentials.json /var/local/fpp/nethesis
cp credentials.p8 /var/local/fpp/nethesis
echo LISTEN=127.0.0.1:9191 > /var/local/fpp/nethesis/env
echo APPLE_TEAM_ID=XXXXXXXXXX >> /var/local/fpp/nethesis/env
APPLE_KEY_ID=YYYYYYYYYY >> /var/local/fpp/nethesis/env
echo APPLE_TOPIC=it.nethesis.nethcti3.voip >> /var/local/fpp/nethesis/env
echo APPLE_ENVIRONMENT=production >> /var/local/fpp/nethesis/env
echo INSTANCE_TOKEN=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx >> /var/local/fpp/nethesis/env
chown -R nobody:nobody /var/local/fpp/nethesis
systemctl enable --now fpp@nethesis
```

You are going to need an instance for each app to notify.

Start traefik:
```
cd deploy
podman run --network=host --name traefik --rm -v $PWD/traefik.yml:/etc/traefik/traefik.yml  -v $PWD/dynamic.yml:/etc/traefik/dynamic.yml traefik:v2.10 
```

Send a notification using curl through Traefik, example:
```
curl -H "Accept: application/json" https://<systemid>:<secret>@dev.gs.nethserver.net/nethesis/send --data '{ ... }'
```
