package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	listenerAddress = "localhost:31337"
)

var (
	// example https://sysdig.com/blog/prometheus-metrics/
	gauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "lightheus",
			Name:      "test_gauge",
			Help:      "This is a test gauge",
		})
)

func main() {
	http.Handle("/metrics/prometheus", promhttp.Handler())

	prometheus.MustRegister(gauge)

	rand.Seed(time.Now().Unix())

	go func() {
		for {
			gauge.Add(rand.Float64()*15 - 5)
			time.Sleep(time.Second)
		}
	}()

	log.Fatal(http.ListenAndServe(listenerAddress, nil))
}

func handleStuff(w http.ResponseWriter, r *http.Request) {

}
