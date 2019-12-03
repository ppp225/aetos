package main

import (
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
    homepage:
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
    otherpage:
      filepath: lighthouse.json
      labels:
        host: test.com
        path: /other
        strategy: mobile
      metric:
        performance:
          help: This is the lighthouse performance score
          path: categories.performance.score
        pwa:
          help: This is the lighthouse pwa score
          path: categories.pwa.score
  lightheus2:
    stuff:
      filepath: lighthouse.json
      metric:
        performance:
          help: This is the lighthouse performance score
          path: categories.performance.score
`

type config struct {
	Namespace map[string]map[string]filegroup `json:"namespace" validate:"required"` // this double map encodes (namespace ->) lightheus -> group -> (group)
	Address   string                          `json:"address" validate:"required"`
}

type filegroup struct {
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
	for _, n := range cfg.Namespace {
		for _, g := range n {
			if err := validate.Struct(g); err != nil {
				log.Fatal(err)
			}
		}
	}
}

var (
	namespaces = make([]namespace, 0)
)

type namespace struct {
	Name   string
	Groups []group
	Gauges []gauge
}
type group struct {
	Name     string
	FilePath string
	Labels   prometheus.Labels
}
type gauge struct {
	GaugeVec *prometheus.GaugeVec
	Path     string
}

func initialize(cfg *config) {
	for nn, n := range cfg.Namespace {
		spacex := namespace{Name: nn}
		for gn, g := range n {
			groupx := group{Name: gn, FilePath: g.FilePath, Labels: g.Labels}
			for mn, m := range g.Metric {
				labelKeys := make([]string, 0, len(g.Labels))
				for k := range g.Labels {
					labelKeys = append(labelKeys, k)
				}
				promGauge := prometheus.NewGaugeVec(
					prometheus.GaugeOpts{
						Namespace: nn,
						Name:      mn,
						Help:      m.Help,
					},
					labelKeys,
				)

				if err := prometheus.Register(promGauge); err != nil {
					// log.Printf("duplicate gauge (expected) TODO: chage initializing logic name=%s_%s\n", nn, mn)
					continue
				}
				log.Printf("registering gauge name=%s_%s\n", nn, mn)

				gauge := gauge{
					GaugeVec: promGauge,
					Path:     m.Path,
				}
				spacex.Gauges = append(spacex.Gauges, gauge)
			}
			spacex.Groups = append(spacex.Groups, groupx)
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
				for _, g := range n.Groups {
					data := unjson.LoadFile(g.FilePath)
					for _, gauge := range n.Gauges {
						value := unjson.Get(data, gauge.Path).(float64)
						gauge.GaugeVec.With(g.Labels).Set(value)
					}
				}
			}
			time.Sleep(time.Second * 10)
		}
	}()

	log.Println("Starting listening on http://" + cfg.Address + "/metrics/prometheus")
	log.Fatal(http.ListenAndServe(cfg.Address, nil))
}
