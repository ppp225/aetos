package main

import (
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator"
	"github.com/ppp225/unjson"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

// example https://sysdig.com/blog/prometheus-metrics/

var yml = `
address: localhost:22596
namespace:
  lightheus:
    filepath: lighthouse.json
    labels:
      host: test.com
      path: /
      strategy: mobile
    metric:
      performance:
        help: This is the lighthouse performance score
        path: categories.performance.score
      pwa:
        help: This is the lighthouse pwa score
        path: categories.pwa.score
  lightheus2:
    filepath: lighthouse.json
    metric:
      performance:
        help: This is the lighthouse performance score
        path: categories.performance.score
`

type config struct {
	Namespace map[string]file `json:"namespace" validate:"required,dive"`
	Address   string          `json:"address" validate:"required"`
}
type file struct {
	FilePath string            `json:"filepath" validate:"required"`
	Metric   map[string]metric `json:"metric" validate:"required,dive"`
	Labels   map[string]string `json:"labels" validate:""`
}

type metric struct {
	Help string `json:"help" validate:"required"`
	Path string `json:"path" validate:"required"`
	Type string `type:"path" validate:"len=0"` // not supported currently
}

func getConfig() *config {
	var cfg config
	if err := yaml.Unmarshal([]byte(yml), &cfg); err != nil {
		log.Fatal(err)
	}
	validateConfig(&cfg)
	return &cfg
}

func validateConfig(cfg *config) {
	validate := validator.New()
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]

		if name == "-" {
			return ""
		}

		return name
	})

	if err := validate.Struct(cfg); err != nil {
		log.Fatal(err)
	}
}

var (
	namespaces = make([]namespace, 0)
)

type namespace struct {
	Name     string
	FilePath string
	Gauges   []gauge
	Labels   prometheus.Labels
}
type gauge struct {
	GaugeVec *prometheus.GaugeVec
	Path     string
}

func initialize(cfg *config) {
	for nn, n := range cfg.Namespace {
		spacex := namespace{Name: nn, FilePath: n.FilePath, Labels: n.Labels}
		for mn, m := range n.Metric {
			labelKeys := make([]string, 0, len(n.Labels))
			for k := range n.Labels {
				labelKeys = append(labelKeys, k)
			}
			g := prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: nn,
					Name:      mn,
					Help:      m.Help,
				},
				labelKeys)
			prometheus.MustRegister(g)

			gauge := gauge{
				GaugeVec: g,
				Path:     m.Path,
			}
			spacex.Gauges = append(spacex.Gauges, gauge)
		}
		namespaces = append(namespaces, spacex)
	}
}

func main() {
	cfg := getConfig()
	initialize(cfg)

	http.Handle("/metrics/prometheus", promhttp.Handler())

	go func() {
		for {
			for _, n := range namespaces {
				data := unjson.LoadFile(n.FilePath)
				for _, g := range n.Gauges {
					value := unjson.Get(data, g.Path).(float64)
					g.GaugeVec.With(n.Labels).Set(value)
				}
			}
			time.Sleep(time.Second * 10)
		}
	}()

	fmt.Println("Starting listening on http://" + cfg.Address + "/metrics/prometheus")
	log.Fatal(http.ListenAndServe(cfg.Address, nil))
}
