# FPP (what the Fuck Push Proxy)

This proxy receive a notification request and forward it:
- to [Google Firebase Cloud Messaging](https://firebase.google.com/docs/cloud-messaging) for Android phones
- to [Apple APN](https://developer.apple.com/documentation/usernotifications) for iOS phones

It's designed to be used a push notification proxy for [Android NethCTI app](https://github.com/nethesis/nethcti-app-android)
and [iOS NethCTI app](https://github.com/nethesis/nethcti-app-iphone).

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

The server exposes 3 APIs:

- `/ping`: GET, just test if the service is online
- `/register`: POST, register a iOS device. Both the device token and the topic must be a string of 64 hex characters.
  If device token and topic are valid, the server will save the token associated to the given topic.
  Token/topic association will expire after 356 days: applications must be opened at least once a year
  to keep receiving notifications.
  This endpoint must be validated with an header `Instance-Token`, see `INSTANCE_TOKEN` env vaa.
  Parameters:
  - `token`: Apple Device token
  - `topic`: Unique topic to identify the mobile device. The topic is the sha256sum of the token received by the client
- `/send`: POST, send a wake-up notification to a device
  Parameters:
  - `type`: Notification type, can be `apple` or `firebase`. If `type` is `apple`, the
    iOS device should already have been registered: the server will wake up the device token
    corresponding to the given `topic`
  - `topic`: Unique topic to identify the mobile device. The topic is the sha256sum of the token received by the client
    after the login
  - `uuid`: Flexisip transaction identifier
  - `call-id`: Asterisk call identifier
  - `from-uri`: Caller SIP URI
  - `display-name`: Caller display name

The server can be configured using the following environment variables:
- `GOOGLE_APPLICATION_CREDENTIALS`: (required) path of the service account JSON file
- `LISTEN`: (optional) listen address and port, default is `127.0.0.1:8080`
- `APPLE_TEAM_ID`: Apple Team ID for p8 credentials
- `APPLE_KEY_ID`: Apple Key ID for p8 credentials
- `APPLE_TOPIC`: topic for Apple APN, like `it.nethesis.nethcti3.voip` (note the `.voip` suffix)
- `APPLE_ENVIRONMENT` can be `production` or `sandbox`
- `DB_PATH`: path for [Badger](https://github.com/dgraph-io/badger) database
- `INSTANCE_TOKEN`: a SHA25sum hash string representing a token for this instance.
   Requests to `/register` token are validated against this token.
   This token must be compiled inside each mobile app

Send a notification using curl, example:
```
curl -H "Accept: application/json" http://localhost:8080/send \
  --data '{"type": "firebase", "topic": "b62aabfc0699e752fdbfda027433342f7bd20715da07049956d0daf20e34f326", "uuid": "550e8400-e29b-41d4-a716-446655440000", "call-id": "000001", "display-name": "Test user", "from-uri": "sip:401@127.0.0.1"}'
```

Register an iOS device, example:
```
curl -H "Accept: application/json" -H "Instance-Token: 8dc657c146dc410790adb71d0be62c591b9bfda9c1c889972c0d2a095b71f733" "http://localhost:8080/register \
  --data '{"token": "A6F31E46858BF045475A529F709D781B8D48507DFFDCEBBB1DCEF907FD58AC05", "topic": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}'
```

## Logging

Each requess is logged to standard error in CSV format to ease future data analisys.
The standard error is redirect to syslog using systemd unit.

Each line can have 2 different formats.

### Send

Log of `send` requests has the following format:
```
datetime_rfc3339,type,result,response,topic,callid,uuid
```

Example of successfull request:
```
2023-07-12T09:16:15Z,apple,success,projects/nethcti-f0ff1/messages/1789059028385963799,a21cec13ef9cc70f2cf56d7c696476d6247b2a6e690bb8251cdaf559771f8529,1234,334455
```

Example of errored requst:
```
2023-07-12T09:16:18Z,firebase,error,invalid topic,a21cec13ef9cc70f2cf56d7c696476d6247b2a6e690bb8251cdaf559771f8529,1234,334455
```


### Register

Log of `register` request has the following format:
```
datetime_rfc3339,type,result,response,token,topic
```

Example of successfull request:
```
```

Example of errored request:
```
```

## Build and deploy

The deploy procedure should:
- configure 2 fpp instances for every branding: one for production and one for sandbox
- configure a Traefik instance to authenticate the requests and forward them to the right fpp instance:
  - `ping` and `send` endopoints must be authenticated by Trafik using `my.nethesis.it`
  - `register` endpoint should be not authenticated by Traefik


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
echo LISTEN=127.0.0.1:9191 > /var/local/fpp/nethesis/env
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
curl -H "Accept: application/json" https://<systemid>:<secret>@dev.gs.nethserver.net/nethesis/send
  --data '{"topic": "testmst%nethctiapp.nethserver.net", "uuid": "550e8400-e29b-41d4-a716-446655440000", "call-id": "000001"}'
```
