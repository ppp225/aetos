package aetos

import (
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator"
	log "github.com/ppp225/lvlog"
	"github.com/ppp225/unjson"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

const pkg = "aetos"

// example https://sysdig.com/blog/prometheus-metrics/

type config struct {
	Groups      map[string]*namespaceGroup `yaml:"groups" validate:"required,dive"`
	Address     string                     `yaml:"address" validate:"required"`
	MetricsPath string                     `yaml:"metrics_path" validate:""`
}

type namespaceGroup struct {
	Namespace string            `yaml:"namespace" validate:""` // allows overriding namespace name; default is group map key
	Metrics   map[string]metric `yaml:"metrics" validate:"required,dive"`
	Labels    map[string]string `yaml:"labels" validate:""`
	Files     map[string]File   `yaml:"files" validate:"required,dive"`
}
type metric struct {
	Help string `yaml:"help" validate:"required"`
	Path string `yaml:"path" validate:"required"`
	Type string `yaml:"type" validate:"len=0"` // not supported currently
}
type File struct {
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

func loadConfig(path string) *config {
	ymlBytes := loadFile(path)
	var cfg config
	if err := yaml.Unmarshal(ymlBytes, &cfg); err != nil {
		log.Fatal(err)
	}
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
	Name     string
	GaugeVec *prometheus.GaugeVec
	Path     string
}

func initialize(cfg *config) []namespace {
	namespaces := make([]namespace, 0)

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
				log.Errorf("pkg=%s msg=%q, gauge=\"%s_%s\", error=%q\n", pkg, "registering gauge failed", nn, mn, err)
				continue
			}
			log.Infof("pkg=%s msg=%q name=%s_%s\n", pkg, "registered gauge", nn, mn)

			gauge := gauge{
				GaugeVec: promGauge,
				Path:     m.Path,
				Name:     mn,
			}
			spacex.Gauges = append(spacex.Gauges, gauge)
		}
		namespaces = append(namespaces, spacex)
	}
	return namespaces
}

// Aetos represents Aetos instance
type Aetos struct {
	cfg        *config
	namespaces []namespace
}

// New creates new eagle
func New(configPath string) *Aetos {
	cfg := loadConfig(configPath)
	validateConfig(cfg)
	ns := initialize(cfg)

	return &Aetos{
		cfg:        cfg,
		namespaces: ns,
	}
}

// NewBaseWithFiles creates new Aetos instance, but only single namespace group is allowed,
// and files are supplied from external source
func NewBaseWithFiles(baseConfigPath string, files []File) *Aetos {
	cfg := loadConfig(baseConfigPath)
	if len(cfg.Groups) != 1 {
		log.Fatal("Only single group supported in this initializer")
	}
	for k := range cfg.Groups {
		cfg.Groups[k].Files = make(map[string]File)
		for _, f := range files {
			cfg.Groups[k].Files[f.FilePath] = f
		}
	}
	validateConfig(cfg)
	ns := initialize(cfg)

	return &Aetos{
		cfg:        cfg,
		namespaces: ns,
	}
}

func (v *Aetos) Debug() {
	log.SetLevel(log.ALL)
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
			for _, n := range v.namespaces {
				for _, g := range n.Groups {
					data := unjson.LoadFile(g.FilePath)
					for _, gauge := range n.Gauges {
						value, ok := unjson.Get(data, gauge.Path).(float64)
						if !ok {
							log.Debugf("pkg=%s msg=%q json_path=%q\n", pkg, "couldn't parse json_path as float64, setting it to 0.0", gauge.Path)
							value = 0.0
						}
						log.Tracef("pkg=%s msg=%q gauge=\"%s_%s\" group=%q file=%q path=%q value=\"%v\" labels=%q\n", pkg, "updating gauge", n.Name, gauge.Name, g.Name, g.FilePath, gauge.Path, value, g.Labels)
						gauge.GaugeVec.With(g.Labels).Set(value)
					}
				}
			}
			time.Sleep(time.Second * 10)
		}
	}()

	log.Infof("pkg=%s msg=\"Starting listening on http://%s%s\"\n", pkg, cfg.Address, metricsPath)
	log.Fatal("pkg=aetos", http.ListenAndServe(cfg.Address, nil))
}
