package main

import (
	"os"
	"fmt"
	"time"
	"context"
	"net/http"
	"encoding/csv"

	"github.com/gin-gonic/gin"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
)

var app *firebase.App
var ctx context.Context
var client *messaging.Client

type Response struct {
	Message string `json:"message"`
}

type Notification struct {
	CallId string `json:"call-id"`
	Uuid string `json:"uuid"`
	Topic string `json:"topic"`
	Type string `json:"type"`
	FromUri string `json:"from-uri"`
	DisplayName string `json:"display-name"`
}

func ping(c *gin.Context) {
	c.JSON(http.StatusOK, Response{Message: "pong"})
}

func audit(result string, response string, notification *Notification) {
	now := time.Now().Format(time.RFC3339)

	w := csv.NewWriter(os.Stdout)
	record := []string{now, notification.Type, result, response, notification.Topic, notification.CallId, notification.Uuid}
	if err := w.Write(record); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing record to csv:", err)
	}
	w.Flush()
}

func send(c *gin.Context) {
	var notification Notification
	var message *messaging.Message

	if err := c.BindJSON(&notification); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	if notification.Type == "apple" {
		badge := 0
		alert := &messaging.ApsAlert {
			Title: notification.CallId,
			Body: notification.Uuid,
			LocKey: "call",
		}
		payload := &messaging.APNSPayload {
			Aps: &messaging.Aps{
				Badge: &badge,
				ContentAvailable: true,
				CustomData: map[string]interface{}{
			//		"loc-key": "call",
			//		"loc-args": []string{},
					"call-id": notification.CallId,
					"uuid": notification.Uuid,
					"send-time": uint(time.Now().Unix()),
				},
				Alert: alert,
			},
			CustomData: map[string]interface{}{
				"from-uri": notification.FromUri,
				"display-name": notification.DisplayName,
				"pn_ttls": 0,
				"customPayload": struct{}{},
			},
		}

		message = &messaging.Message{
			Data: map[string]string{ "call-id": notification.CallId, "uuid": notification.Uuid },
			APNS: &messaging.APNSConfig{
				Headers: map[string]string{
					"apns-priority": "10",
					"apns-push-type": "voip",
					"apns-topic": "it.nethesis.nethcti3.voip",
				},
				Payload: payload,
			},
			Topic: notification.Topic,
		}
	} else if notification.Type == "firebase" {
		message = &messaging.Message{
			Data: map[string]string{ "call-id": notification.CallId, "uuid": notification.Uuid },
			Topic: notification.Topic,
		}
	} else {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid notification type"})
		return
	}
	response, err := client.Send(ctx, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
		audit("error", err.Error(), &notification)
		return
	}

	audit("success", response, &notification)
	c.JSON(http.StatusOK, Response{Message: response})
}

func initFirebase() (app *firebase.App) {
	var err error
	app, err = firebase.NewApp(context.Background(), nil)

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error initializing app: ", err.Error())
		os.Exit(1)
	}
        ctx = context.Background()
	client, err = app.Messaging(ctx)

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error initializing messaging client: ", err.Error())
		os.Exit(2)
	}

	return app
}


func main() {
	initFirebase()

	router := gin.Default()
	router.GET("/ping", ping)
	router.POST("/send", send)

	listen := os.Getenv("LISTEN")
	if len(listen) == 0 {
		listen = "127.0.0.1:8080"
	}

	router.Run(listen)
}
