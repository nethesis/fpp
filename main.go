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
}

func ping(c *gin.Context) {
	c.JSON(http.StatusOK, Response{Message: "pong"})
}

func audit(result string, response string, notification *Notification) {
	now := time.Now().Format(time.RFC3339)

	w := csv.NewWriter(os.Stdout)
	record := []string{now, result, response, notification.Topic, notification.CallId, notification.Uuid}
	if err := w.Write(record); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing record to csv:", err)
	}
	w.Flush()
}

func send(c *gin.Context) {
	var notification Notification

	if err := c.BindJSON(&notification); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	message := &messaging.Message{
		Data: map[string]string{ "call-id": notification.CallId, "uuid": notification.Uuid },
		Topic: notification.Topic,
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
