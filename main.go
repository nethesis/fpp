package main

import (
	"context"
	"fmt"
	"net/http"

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

func send(c *gin.Context) {
	var notification Notification

	if err := c.BindJSON(&notification); err != nil {
        	return
    	}

	message := &messaging.Message{
		Data: map[string]string{ "call-id": notification.CallId, "uuid": notification.Uuid },
        	Topic: notification.Topic,
	}

	response, err := client.Send(ctx, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
		fmt.Println("Error:", err.Error())
	}

	fmt.Println("Success:", response)
	c.JSON(http.StatusOK, Response{Message: response})
}

func initFarebase() (app *firebase.App) {
	app, err := firebase.NewApp(context.Background(), nil)

	if err != nil {
		fmt.Errorf("error initializing app: %v", err)
		return nil
	}

	return app
}

func main() {
	app = initFarebase()
	ctx = context.Background()
	client, _ = app.Messaging(ctx)

	//if err != nil {
        //	log.Fatalln(err)
	//}

	router := gin.Default()
	router.GET("/ping", ping)
	router.POST("/send", send)

	router.Run()
}

