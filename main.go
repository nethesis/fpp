package main

import (
	"os"
	"fmt"
	"time"
	"regexp"
	"context"
	"net/http"
	"encoding/csv"

	"github.com/gin-gonic/gin"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/go-co-op/gocron"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
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

/** Structures definition **/

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

type Registration struct {
	Token string `json:"token"`
	Topic string `json:"topic"`
}

type Metrics struct {
	RegisteredDevices prometheus.Gauge
	TotalSendCount prometheus.Counter
	APNSuccessCount prometheus.Counter
	APNErrorCount prometheus.Counter
	FirebaseSuccessCount prometheus.Counter
	FirebaseErrorCount prometheus.Counter
}

/** Audit and metrics functions **/

func countRegisteredDevices() float64{
	var count = 0
	db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if !it.Item().IsDeletedOrExpired() {
				count++
			}
		}
		return nil
	})

	return float64(count)
}

func audit(record []string) {
	now := time.Now().Format(time.RFC3339)
	w := csv.NewWriter(os.Stdout)
	if err := w.Write(append([]string{now}, record...)); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing record to csv:", err)
	}
	w.Flush()

	// Update metrics
	if record[0] == "send" {
		metrics.TotalSendCount.Inc()
		if record[1] == "apple" {
			if record[2] == "success" {
				metrics.APNSuccessCount.Inc()
			} else {
				metrics.APNErrorCount.Inc()
			}
		} else if record[1] == "firebase" {
			if record[2] == "success" {
				metrics.FirebaseSuccessCount.Inc()
			} else {
				metrics.FirebaseErrorCount.Inc()
			}
		}
	} else if record[0] == "register" {
		metrics.RegisteredDevices.Set(countRegisteredDevices())
	}
}

func auditSend(result string, response string, notification *Notification) {
	audit([]string{"send", notification.Type, result, response, notification.Topic, notification.CallId, notification.Uuid})
}

func auditRegister(result string, rtype string, response string, token string, topic string) {
	audit([]string{"register", rtype, result, response, token, topic})
}

func auditDeregister(result string, rtype string, response string, token string, topic string) {
	audit([]string{"deregister", rtype, result, response, token, topic})
}

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
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{Message: "No device token found"})
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

func initDB() (* badger.DB){
	dbPath := os.Getenv("DB_PATH")
	if len(dbPath) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing DB: invalid db path")
		os.Exit(4)
	}

	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error initializing DB: ", err.Error())
		os.Exit(5)
	}

	// Setup db cleanup job
	cleanupJob := gocron.NewScheduler(time.UTC)
	cleanupJob.Every(2).Seconds().Do(func() {
		err := db.RunValueLogGC(0.5)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error on DB cleanup: ", err.Error())
		}
	})
	return db
}

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
