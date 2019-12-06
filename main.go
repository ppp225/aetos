package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
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

type config struct {
	Groups  map[string]namespaceGroup `json:"groups" validate:"required,dive"`
	Address string                    `json:"address" validate:"required"`
}

type namespaceGroup struct {
	Namespace string            `json:"namespace" validate:""` // allows overriding namespace name; default is group map key
	Metrics   map[string]metric `json:"metrics" validate:"required,dive"`
	Labels    map[string]string `json:"labels" validate:""`
	Files     map[string]file   `json:"files" validate:"required,dive"`
}
type metric struct {
	Help string `json:"help" validate:"required"`
	Path string `json:"path" validate:"required"`
	Type string `type:"type" validate:"len=0"` // not supported currently
}
type file struct {
	FilePath string            `json:"filepath" validate:"required"`
	Labels   map[string]string `json:"labels" validate:""`
}

func loadFile(filename string) []byte {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	bytes, _ := ioutil.ReadAll(file)
	return bytes
}

func getConfig() *config {
	ymlBytes := loadFile("config.yml")
	var cfg config
	if err := yaml.Unmarshal(ymlBytes, &cfg); err != nil {
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
	Name   string
	Groups []filegroup
	Gauges []gauge
}
type filegroup struct {
	Name     string
	FilePath string
	Labels   prometheus.Labels
}
type gauge struct {
	GaugeVec *prometheus.GaugeVec
	Path     string
}

func initialize(cfg *config) {
	for nn, n := range cfg.Groups {
		// create namespace; (check name override)
		if len(n.Namespace) > 0 {
			nn = n.Namespace
		}
		spacex := namespace{Name: nn}
		// create file-groups
		for gn, g := range n.Files {
			for k, v := range n.Labels {
				g.Labels[k] = v
			}
			groupx := filegroup{Name: gn, FilePath: g.FilePath, Labels: g.Labels}
			spacex.Groups = append(spacex.Groups, groupx)
		}
		// generate and validate labels
		labelKeys := make([]string, 0, len(spacex.Groups[0].Labels))
		for k := range spacex.Groups[0].Labels {
			labelKeys = append(labelKeys, k)
		}
		// create metrics
		for mn, m := range n.Metrics {
			promGauge := prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: nn,
					Name:      mn,
					Help:      m.Help,
				},
				labelKeys,
			)

			if err := prometheus.Register(promGauge); err != nil {
				log.Printf("ERROR: duplicate gauge, name=%s_%s\n", nn, mn)
				continue
			}
			log.Printf("registering gauge name=%s_%s\n", nn, mn)

			gauge := gauge{
				GaugeVec: promGauge,
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
