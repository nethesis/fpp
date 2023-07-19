package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

/** Metrics functions and structs **/

type Metrics struct {
	RegisteredAPNDevices      prometheus.Gauge
	RegisteredFirebaseDevices prometheus.Gauge
	TotalSendCount            prometheus.Counter
	APNSuccessCount           prometheus.Counter
	APNErrorCount             prometheus.Counter
	FirebaseSuccessCount      prometheus.Counter
	FirebaseErrorCount        prometheus.Counter
}

func updateDeviceMetrics() {
	apnCount, fbCount := countRegisteredDevices()
	metrics.RegisteredAPNDevices.Set(apnCount)
	metrics.RegisteredFirebaseDevices.Set(fbCount)
}

func updateMetrics(record []string) {
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
	} else if record[0] == "register" || record[0] == "deregister" {
		updateDeviceMetrics()
	}

}

func initMetrics() (*Metrics, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := &Metrics{
		RegisteredAPNDevices: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fpp_registered_apn_devices",
			Help: "Number of registered Apple APN devices.",
		}),
		RegisteredFirebaseDevices: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fpp_registered_firebase_devices",
			Help: "Number of registered Google Firebase devices.",
		}),
		TotalSendCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fpp_total_send_count",
			Help: "Number of sent notifications.",
		}),
		APNSuccessCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fpp_apn_success_count",
			Help: "Number of successfull Apple APN notifications.",
		}),
		APNErrorCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fpp_apn_error_count",
			Help: "Number of errored Apple APN notifications.",
		}),
		FirebaseSuccessCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fpp_firebase_success_count",
			Help: "Number of successfull Google Firebase notifications.",
		}),
		FirebaseErrorCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fpp_firebase_error_count",
			Help: "Number of errored Google Firebase notifications.",
		}),
	}
	reg.MustRegister(m.RegisteredAPNDevices)
	reg.MustRegister(m.RegisteredFirebaseDevices)
	reg.MustRegister(m.TotalSendCount)
	reg.MustRegister(m.APNSuccessCount)
	reg.MustRegister(m.APNErrorCount)
	reg.MustRegister(m.FirebaseSuccessCount)
	reg.MustRegister(m.FirebaseErrorCount)

	return m, reg
}
