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

