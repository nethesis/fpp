package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"firebase.google.com/go/messaging"

	"github.com/sideshow/apns2"

	badger "github.com/dgraph-io/badger/v4"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var apnClient *apns2.Client
var apnTopic string
var fbClient *messaging.Client
var ctx context.Context
var db *badger.DB
var ticker *time.Ticker
var instanceToken string
var metrics *Metrics

/** Gin middleware **/

func InstanceTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Instance-Token")

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Instance-Token header required"})
			return
		}

		if token != instanceToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Instance-Token header"})
			return
		}

		c.Next()
	}
}

/** Utils **/

func isAscii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func validateRegistration(registration Registration) error {
	topicReg, _ := regexp.Compile("^[0-9a-fA-F]+$")
	spaceReg, _ := regexp.Compile("\\s+")

	if registration.Type != "apple" && registration.Type != "firebase" {
		return errors.New("Invalid type")
	}

	// Topic is a SHA256 hash
	if !topicReg.MatchString(registration.Topic) || len(registration.Topic) != 64 {
		return errors.New("Invalid topic")
	}

	// Token format is not known and can change over time
	// Very lazy validation: it must be a non-empty ASCII string without spaces
	if !isAscii(registration.Token) || spaceReg.MatchString(registration.Token) || len(registration.Token) < 1 {
		return errors.New("Invalid token")
	}

	return nil
}

/** Gin routes **/

func register(c *gin.Context) {
	var registration Registration

	if err := c.BindJSON(&registration); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	// Check registration format
	if err := validateRegistration(registration); err != nil {
		auditRegister("error", registration.Type, err.Error(), registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
		return
	}

	// If the token is valid store <topic>:<device> key/value
	dberr := saveTopicToken(registration.Topic, registration.Token, registration.Type)

	if dberr != nil {
		auditRegister("error", registration.Type, dberr.Error(), registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: dberr.Error()})
		return
	}
	auditRegister("success", registration.Type, "ok", registration.Token, registration.Topic)
	c.JSON(http.StatusOK, Response{Message: "success"})
}

func deregister(c *gin.Context) {
	var registration Registration

	if err := c.BindJSON(&registration); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	// Check registration format
	if err := validateRegistration(registration); err != nil {
		auditRegister("error", registration.Type, err.Error(), registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
		return
	}

	// Check if tuple key/topic matches
	deviceToken, err := getTokenFromTopic(registration.Topic)
	if err != nil || deviceToken != registration.Token {
		auditDeregister("error", registration.Type, "invalid tuple", registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid token/topic tuple"})
		return
	}

	// Delete token/topic tuple
	dberr := deleteTopic(registration.Topic)
	if dberr != nil {
		auditDeregister("error", registration.Type, dberr.Error(), registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: dberr.Error()})
		return
	}

	auditDeregister("success", registration.Type, "ok", registration.Token, registration.Topic)
	c.JSON(http.StatusOK, Response{Message: "success"})
}

func send(c *gin.Context) {
	var notification Notification
	var message *messaging.Message

	if err := c.BindJSON(&notification); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	deviceToken, err := getTokenFromTopic(notification.Topic)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
		return
	}

	if notification.Type == "apple" {
		apnNotif := &apns2.Notification{}
		apnNotif.DeviceToken = deviceToken
		apnNotif.Topic = apnTopic
		apnNotif.PushType = apns2.PushTypeVOIP
		apnNotif.Payload = []byte(fmt.Sprintf(`{
			"aps" : {
				"sound": "",
				"call-id": "%s",
				"uuid": "%s",
				"send-time": "%s"
			},
			"from-uri": "%s",
			"display-naame": "%s",
			"pn_ttl": 100,
			"customPayload": {}
		}
		`, notification.CallId, notification.Uuid, time.Now().Format("2006-01-02 12:32:06"), notification.FromUri, notification.DisplayName))
		// Send voip notification
		res, err := apnClient.Push(apnNotif)
		if res == nil {
			c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
			auditSend("error", err.Error(), &notification)
			return
		}

		if res.StatusCode != 200 {
			// Cleanup stale token
			// 410 response: https://developer.apple.com/library/archive/documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/CommunicatingwithAPNs.html#//apple_ref/doc/uid/TP40008194-CH11-SW15
			if res.StatusCode == 410 {
				deleteTopic(notification.Topic)
			}
			c.JSON(http.StatusInternalServerError, Response{Message: res.Reason})
			auditSend("error", res.Reason, &notification)
			return
		}

		// Extend topic expiration
		// Ignore error here. Worst case: the topic expires after the TTL
		saveTopicToken(notification.Topic, deviceToken, notification.Type)

		auditSend("success", res.ApnsID, &notification)
		c.JSON(http.StatusOK, Response{Message: res.ApnsID})
		return
	} else if notification.Type == "firebase" {
		message = &messaging.Message{
			Android: &messaging.AndroidConfig{
				Priority: "high",
			},
			// Android notification does not require any info: it is used only to wake up the app
			// Just send a timestamp to make sure notification is always different
			Data:  map[string]string{"timestamp": strconv.FormatInt(time.Now().UnixMilli(), 10)},
			Token: deviceToken,
		}
		response, err := fbClient.Send(ctx, message)
		if err != nil {
			// Clean stale token
			// 404 response: https://firebase.google.com/docs/reference/fcm/rest/v1/ErrorCode
			if strings.Contains(err.Error(), "registration-token-not-registered") {
				deleteTopic(notification.Topic)
			}
			c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
			auditSend("error", err.Error(), &notification)
			return
		}

		// Extend topic expiration
		// Ignore error here. Worst case: the topic expires after the TTL
		saveTopicToken(notification.Topic, deviceToken, notification.Type)

		auditSend("success", response, &notification)
		c.JSON(http.StatusOK, Response{Message: response})
		return
	} else {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid notification type"})
		return
	}
}

/** Initialization functions **/

func initInstance() {
	instanceToken = os.Getenv("INSTANCE_TOKEN")
	if len(instanceToken) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing instance token: empty environment variable")
		os.Exit(6)
	}
}

func sigHandler(signal os.Signal) {
	if signal == syscall.SIGTERM || signal == syscall.SIGINT || signal == syscall.SIGKILL || signal == os.Interrupt {
		err := db.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error closing the db: ", err.Error())
			os.Exit(7)
		}
		os.Exit(0)
	}
}

func initSignals() {
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel)

	go func() {
		for {
			s := <-sigChannel
			sigHandler(s)
		}
	}()
}

/** Application handler **/

func main() {
	// Initialize signal handling
	initSignals()
	// Connect to Firebase for Android notifications
	initFirebase()
	// Connect to Apple APN for iOS notifications
	initAPN()
	// Initialize DB to store iOS device token
	db = initDB()
	defer db.Close()
	// Initialize instance API key
	initInstance()
	// Initialize signals
	initSignals()
	// Initialize metrics
	var reg *prometheus.Registry
	metrics, reg = initMetrics()
	promHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
	// Update db metrics
	updateDeviceMetrics()

	router := gin.Default()
	router.GET("/metrics", gin.WrapH(promHandler))
	router.POST("/send", send)
	router.POST("/register", InstanceTokenAuth(), register)
	router.POST("/deregister", InstanceTokenAuth(), deregister)

	listen := os.Getenv("LISTEN")
	if len(listen) == 0 {
		listen = "127.0.0.1:8080"
	}

	router.Run(listen)
}
