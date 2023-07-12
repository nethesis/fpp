# FPP (Firebase Push Proxy)

This proxy receive a notification request and forward it to [Google Firebase Cloud Messaging](https://firebase.google.com/docs/cloud-messaging).
It's designed to be used a push notification proxy for [Android NethCTI app](https://github.com/nethesis/nethcti-app-android)
and [iOS NethCTI app](https://github.com/nethesis/nethcti-app-iphone).

Before starting the server, make sure to get a valid [service account in JSON format](https://firebase.google.com/docs/admin/setup).

## Usage

Download a service account and save it to a file named `credentials.json`, then execute the server:
```
GOOGLE_APPLICATION_CREDENTIALS="./credentials.json" ./fpp
```

The server exposes 2 APIs:

- `/ping`: GET, just test if the service is online
- `/send`: POST, send a wake-up notification to a device via Firebase
  Parameters:
  - `topic`: Unique topic to identify the mobile device. The topic is the sha256sum of the token received by the client
  after the login
  - `uuid`: Flexisip transaction identifier
  - `call-id`: Asterisk call identifier

The server can be configured using the following environment variables:
- `GOOGLE_APPLICATION_CREDENTIALS`: (required) path of the service account JSON file
- `LISTEN`: (optional) listen address and port, default is `127.0.0.1:8080`


Send a notification using curl, example:
```
curl -H "Accept: application/json" http://localhost:8080/send \
  --data '{"topic": "testmst%nethctiapp.nethserver.net", "uuid": "550e8400-e29b-41d4-a716-446655440000", "call-id": "000001"}'
```

## Logging

Each requess is logged to standard error in CSV format to ease future data analisys.
The standard error is redirect to syslog using systemd unit.

Each line has the following format:
```
datetime_rfc3339,result,response,topic,callid,uuid
```

Example of success request:
```
2023-07-12T09:16:15Z,success,projects/nethcti-f0ff1/messages/1789059028385963799,a21cec13ef9cc70f2cf56d7c696476d6247b2a6e690bb8251cdaf559771f8529,1234,334455
```

Example of errored requst:
```
2023-07-12T09:16:18Z,error,invalid topic,a21cec13ef9cc70f2cf56d7c696476d6247b2a6e690bb8251cdaf559771f8529,1234,334455
```

## Build and deploy

The deploy procedure will:
- configure a fpp instance for every branding
- configure a Traefik instance to authenticate the requests and forward them to the right fpp instance


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
curl -H "Accept: application/json" https://<systemid>:<secret>@dev.gs.nethserver.net/nethesis/ping
  --data '{"topic": "testmst%nethctiapp.nethserver.net", "uuid": "550e8400-e29b-41d4-a716-446655440000", "call-id": "000001"}'
```
