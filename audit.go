package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"time"
)

/** Audit functions **/

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
