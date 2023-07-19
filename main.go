package main

import (
	"os"
	"fmt"
	"time"
	"errors"
	"regexp"
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"firebase.google.com/go/messaging"

	"github.com/sideshow/apns2"

	badger "github.com/dgraph-io/badger/v4"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/collectors"
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

/** Gin routes **/

func ping(c *gin.Context) {
	c.JSON(http.StatusOK, Response{Message: "pong"})
}


func register(c *gin.Context) {
	var registration Registration

	if err := c.BindJSON(&registration); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	// Check token and topic format
	r, _ := regexp.Compile("^[0-9a-fA-F]+$")
	if ! r.MatchString(registration.Token) || len(registration.Token) != 64 {
		auditRegister("error", "apple", "invalid token", registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid token"})
		return
	}
	if ! r.MatchString(registration.Topic) || len(registration.Topic) != 64 {
		auditRegister("error", "apple", "invalid topic", registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid topic"})
		return
	}


	// If the token is valid store <topic>:<device> key/value
	// Set a TTL of one year, the TTL is updated on each use
	dberr := db.Update(func(txn *badger.Txn) error {
		record := badger.NewEntry([]byte(registration.Topic), []byte(registration.Token)).WithTTL(24 * 365 * time.Hour)
		err := txn.SetEntry(record)
		return err
	})

	if dberr != nil {
		auditRegister("error", "apple", dberr.Error(), registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: dberr.Error()})
		return
	}
	auditRegister("success", "apple", "ok", registration.Token, registration.Topic)
	c.JSON(http.StatusOK, Response{Message: "success"})
}

func deregister(c *gin.Context) {
	var registration Registration
	var deviceToken []byte

	if err := c.BindJSON(&registration); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	// Check token and topic format
	r, _ := regexp.Compile("^[0-9a-fA-F]+$")
	if ! r.MatchString(registration.Token) || len(registration.Token) != 64 {
		auditDeregister("error", "apple", "invalid token", registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid token"})
		return
	}
	if ! r.MatchString(registration.Topic) || len(registration.Topic) != 64 {
		auditDeregister("error", "apple", "invalid topic", registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid topic"})
		return
	}

	// Check if tuple key/topic matches
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(registration.Topic))

		if err != nil {
			auditDeregister("error", "apple", "not topic found", registration.Token, registration.Topic)
			c.JSON(http.StatusInternalServerError, Response{Message: "No topic found"})
			return err
		}
		ierr := item.Value(func(val []byte) error {
			deviceToken = make([]byte, len(val))
			copy(deviceToken, val)
			return nil
		})
		return ierr
	})
	if err != nil || string(deviceToken[:]) != registration.Token {
		auditDeregister("error", "apple", "invalid tuple", registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid token/topic tuple"})
		return
	}

	// Delete token/topic tuple
	dberr := db.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(registration.Topic))
		return err
	})

	if dberr != nil {
		auditDeregister("error", "apple", dberr.Error(), registration.Token, registration.Topic)
		c.JSON(http.StatusInternalServerError, Response{Message: dberr.Error()})
		return
	}
	auditDeregister("success", "apple", "ok", registration.Token, registration.Topic)
	c.JSON(http.StatusOK, Response{Message: "success"})
}


func send(c *gin.Context) {
	var notification Notification
	var message *messaging.Message

	if err := c.BindJSON(&notification); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid parameters"})
		return
	}

	if notification.Type == "apple" {
		var deviceToken []byte
		err := db.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(notification.Topic))

			if err != nil {
				return errors.New("No topic found")
			}
			if item.IsDeletedOrExpired() {
				return errors.New("Topic deleted or expired")
			}

			ierr := item.Value(func(val []byte) error {
				deviceToken = make([]byte, len(val))
				copy(deviceToken, val)
				return nil
			})
			return ierr
		})
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
			return
		}
		apnNotif := &apns2.Notification{}
		apnNotif.DeviceToken = string(deviceToken)
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
			c.JSON(http.StatusInternalServerError, Response{Message: res.Reason})
			auditSend("error", res.Reason, &notification)
			return
		}

		// Extend topic expiration
		// Ignore error here. Worst case: the topic expires after one year
		db.Update(func(txn *badger.Txn) error {
			record := badger.NewEntry([]byte(notification.Topic), []byte(deviceToken)).WithTTL(24 * 365 * time.Hour)
			err := txn.SetEntry(record)
			return err
		})

		auditSend("success", res.ApnsID, &notification)
		c.JSON(http.StatusOK, Response{Message: res.ApnsID})
		return
	} else if notification.Type == "firebase" {
		message = &messaging.Message{
			Android: &messaging.AndroidConfig {
				Priority: "high",
			},
			Data: map[string]string{ "call-id": notification.CallId, "uuid": notification.Uuid },
			Topic: notification.Topic,
		}
		response, err := fbClient.Send(ctx, message)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{Message: err.Error()})
			auditSend("error", err.Error(), &notification)
			return
		}

		auditSend("success", response, &notification)
		c.JSON(http.StatusOK, Response{Message: response})
		return
	} else {
		c.JSON(http.StatusInternalServerError, Response{Message: "Invalid notification type"})
		return
	}
}

/** Initialization functions **/

func initInstance(reg prometheus.Registerer) *Metrics {
	instanceToken = os.Getenv("INSTANCE_TOKEN")
	if len(instanceToken) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing instance token: empty environment variable")
		os.Exit(6)
	}

	// Initialize metrics
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := &Metrics{
		RegisteredDevices: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:      "fpp_registered_devices",
			Help:      "Number of registered devices.",
		}),
		TotalSendCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:      "fpp_total_send_count",
			Help:      "Number of sent notifications.",
		}),
		APNSuccessCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:      "fpp_apn_success_count",
			Help:      "Number of successfull Apple APN notifications.",
		}),
		APNErrorCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:      "fpp_apn_error_count",
			Help:      "Number of errored Apple APN notifications.",
		}),
		FirebaseSuccessCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:      "fpp_firebase_success_count",
			Help:      "Number of successfull Google Firebase notifications.",
		}),
		FirebaseErrorCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:      "fpp_firebase_error_count",
			Help:      "Number of errored Google Firebase notifications.",
		}),
	}
	reg.MustRegister(m.RegisteredDevices)
	reg.MustRegister(m.TotalSendCount)
	reg.MustRegister(m.APNSuccessCount)
	reg.MustRegister(m.APNErrorCount)
	reg.MustRegister(m.FirebaseSuccessCount)
	reg.MustRegister(m.FirebaseErrorCount)

	return m
}

/** Application handler **/

func main() {
	// Connect to Firebase for Android notifications
	initFirebase()
	// Connect to Apple APN for iOS notifications
	initAPN()
	// Initialize DB to store iOS device token
	db = initDB()
	defer db.Close()
	// Initialize instance API key and metrics
	reg := prometheus.NewRegistry()
	metrics = initInstance(reg)
	promHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})

	// Update db metrics
	metrics.RegisteredDevices.Set(countRegisteredDevices())

	router := gin.Default()
	router.GET("/ping", ping)
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
