package main

import (
	"encoding/json"
	"fmt"
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
namespace:
  lightheus:
    json: lighthouse.json
    metric:
      performance:
        help: This is the lighthouse performance score
        path: categories.performance.score
      pwa:
        help: This is the lighthouse performance score
        path: categories.pwa.score
  lightheus2:
    json: myjson.json
    metric:
      performance:
        help: This is the lighthouse performance score
        path: categories.performance.score
`

type Config struct {
	Namespace map[string]File
}
type File struct {
	Json   string
	Metric map[string]Metric
}

type Metric struct {
	Help string
	Path string
}

func PrettyPrint(v interface{}) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		fmt.Println(string(b))
	}
	return
}
func unmarshalConfig() {
	var cfg Config
	if err := yaml.Unmarshal([]byte(yml), &cfg); err != nil {
		log.Fatal(err)
	}

	PrettyPrint(cfg)
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
