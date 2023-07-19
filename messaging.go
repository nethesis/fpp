package main

import (
	"context"
	"fmt"
	"os"

	firebase "firebase.google.com/go"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

/** Firebase and APN functions **/

func initFirebase() {
	var fberr error
	app, err := firebase.NewApp(context.Background(), nil)

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error initializing Firebase app: ", err.Error())
		os.Exit(1)
	}
	ctx = context.Background()
	fbClient, fberr = app.Messaging(ctx)

	if fberr != nil {
		fmt.Fprintln(os.Stderr, "Error initializing Firebase messaging client: ", err.Error())
		os.Exit(2)
	}
}

func initAPN() {
	p8 := os.Getenv("APPLE_APPLICATION_CREDENTIALS")
	if len(p8) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing APN client: no p8 file given")
		os.Exit(1)
	}
	authKey, terr := token.AuthKeyFromFile(p8)
	if terr != nil {
		fmt.Fprintln(os.Stderr, "Error initializing APN auth: ", terr.Error())
		os.Exit(2)
	}

	keyId := os.Getenv("APPLE_KEY_ID")
	if len(keyId) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing APN key id: empty key id")
		os.Exit(3)
	}
	teamId := os.Getenv("APPLE_TEAM_ID")
	if len(teamId) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing APN team id: empty team id")
		os.Exit(4)
	}

	apnTopic = os.Getenv("APPLE_TOPIC")
	if len(apnTopic) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing APN topic: empty topic")
		os.Exit(4)
	}

	apnEnv := os.Getenv("APPLE_ENVIRONMENT")
	if len(apnEnv) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing APN env: empty environment")
		os.Exit(4)
	}

	token := &token.Token{
		AuthKey: authKey,
		KeyID:   keyId,
		TeamID:  teamId,
	}

	if apnEnv == "sandbox" {
		apnClient = apns2.NewTokenClient(token).Development()
	} else if apnEnv == "production" {
		apnClient = apns2.NewTokenClient(token).Production()
	} else {
		fmt.Fprintln(os.Stderr, "Error initializing APN env: invalid env value ", apnEnv)
		os.Exit(4)
	}
}
