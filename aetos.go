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
	Groups      map[string]namespaceGroup `yaml:"groups" validate:"required,dive"`
	Address     string                    `yaml:"address" validate:"required"`
	MetricsPath string                    `yaml:"metrics_path" validate:""`
}

type namespaceGroup struct {
	Namespace string            `yaml:"namespace" validate:""` // allows overriding namespace name; default is group map key
	Metrics   map[string]metric `yaml:"metrics" validate:"required,dive"`
	Labels    map[string]string `yaml:"labels" validate:""`
	Files     map[string]file   `yaml:"files" validate:"required,dive"`
}
type metric struct {
	Help string `yaml:"help" validate:"required"`
	Path string `yaml:"path" validate:"required"`
	Type string `yaml:"type" validate:"len=0"` // not supported currently
}
type file struct {
	FilePath string            `yaml:"filepath" validate:"required"`
	Labels   map[string]string `yaml:"labels" validate:""`
}

func loadFile(filename string) []byte {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	bytes, _ := ioutil.ReadAll(file)
	return bytes
}

func getConfig(path string) *config {
	ymlBytes := loadFile(path)
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
		name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]

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
				log.Printf("ERROR: registering gauge failed, name=%s_%s, error=\n", nn, mn, err)
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

// Aetos represents Aetos instance
type Aetos struct {
	cfg *config
}

// New creates new eagle
func New(configPath string) *Aetos {
	cfg := getConfig("aetos.yml")
	initialize(cfg)

	return &Aetos{
		cfg: cfg,
	}
}

func (v *Aetos) Run() {
	cfg := v.cfg

	metricsPath := "/metrics"
	if len(cfg.MetricsPath) > 0 {
		if cfg.MetricsPath[0] == '/' {
			metricsPath = cfg.MetricsPath
		} else {
			metricsPath = "/" + cfg.MetricsPath
		}
	}
	http.Handle(metricsPath, promhttp.Handler())

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

	log.Println("Starting listening on http://" + cfg.Address + metricsPath)
	log.Fatal(http.ListenAndServe(cfg.Address, nil))
}

func main() {
	aetos := New("aetos.yml")
	aetos.Run()
}
