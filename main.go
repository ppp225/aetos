package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
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

var yml = `
lightheus:
  json: myjson.json
  performance:
    help: This is the lighthouse performance score
    path: categories.performance.score
  performance2:
    help: This is the lighthouse performance score
    path: categories.performance.score
lightheus2:
  json: myjson.json
  performance:
    help: This is the lighthouse performance score
    path: categories.performance.score
  performance2:
    help: This is the lighthouse performance score
    path: categories.performance.score
`

type Namespace = map[string]Config

type Config struct {
	Json        string
	Performance Metric
	Metrics     map[string]Metric
}

type Metric struct {
	Help string
	Path string
}

func unmarshalConfig() {
	var cfg Namespace
	if err := yaml.Unmarshal([]byte(yml), &cfg); err != nil {
		log.Fatal(err)
	}

	log.Printf("%+v", cfg)
}

func main() {
	unmarshalConfig()
	return
	// server

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
