# FPP (Firebase Push Proxy)

This proxy receive a notification request and forward it to firebase.

Build the server:
```
go get && go build
```

Execute the server:
```
GOOGLE_APPLICATION_CREDENTIALS="./credentials.json" ./fpp
```

Client example:
```
 curl -H "Accept: application/json" http://165.232.81.5:9191/send --data '{"topic": "testmst%nethctiapp.nethserver.net", "uuid": "xxxx", "call-id": "yyy", "title": "test title", "body": "test body"}'
```

Deploy on Fedora server:
```
setenforce 0
cp fpp /usr/bin/fpp
cp fpp@.service /etc/systemd/system
systemctl daemon-reload

mkdir -p /var/local/fpp/nethesis
cp /root/fpp/credentials.json /var/local/fpp/nethesis
echo LISTEN=127.0.0.1:9191 > /var/local/fpp/nethesis/env
chown -R nobody:nobody /var/local/fpp/nethesis
systemctl enable --now fpp@nethesis
```
