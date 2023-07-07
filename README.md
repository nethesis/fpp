# FPP (Firebase Push Proxy)

This proxy receive a notification request and forward it to firebase.

The proxy requires a service credentials file from Firebase.

Execute the server:
```
GOOGLE_APPLICATION_CREDENTIALS="./credentials.json" ./fpp
```

Client example without Traefik proxy:
```
curl -H "Accept: application/json" http://localhost:9191/send \
  --data '{"topic": "testmst%nethctiapp.nethserver.net", "uuid": "xxxx", "call-id": "yyy", "title": "test title", "body": "test body"}'
```

Client example with Traefik proxy:
```
curl -H "Accept: application/json" http://<system_id>:<secret>@dev.test.nethserver.net/nethesis/send \
  --data '{"topic": "testmst%test.server.org", "uuid": "xxxx", "call-id": "yyy", "title": "Test notification", "body": "tester"}'
```

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
