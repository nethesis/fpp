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
 curl -H "Accept: application/json" http://localhost:8080/send --data '{"topic": "mytopic", "uuid": "xxxx", "call-id": "yyy"}'
```

