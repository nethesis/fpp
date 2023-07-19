package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

/** Metrics functions and structs **/

type Metrics struct {
	RegisteredDevices    prometheus.Gauge
	TotalSendCount       prometheus.Counter
	APNSuccessCount      prometheus.Counter
	APNErrorCount        prometheus.Counter
	FirebaseSuccessCount prometheus.Counter
	FirebaseErrorCount   prometheus.Counter
}

func initMetrics() (*Metrics, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := &Metrics{
		RegisteredDevices: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fpp_registered_devices",
			Help: "Number of registered devices.",
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
	reg.MustRegister(m.RegisteredDevices)
	reg.MustRegister(m.TotalSendCount)
	reg.MustRegister(m.APNSuccessCount)
	reg.MustRegister(m.APNErrorCount)
	reg.MustRegister(m.FirebaseSuccessCount)
	reg.MustRegister(m.FirebaseErrorCount)

	return m, reg
}
