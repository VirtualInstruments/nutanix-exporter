//
// nutanix-exporter
//
// Prometheus Exportewr for Nutanix API
//
// Version: v0.5.1
// Author: Martin Weber <martin.weber@de.clara.net>
// Company: Claranet GmbH
//

package main

import (
	"flag"
	"fmt"
	"net/http"
	"nutanix-exporter/internal/nutanix"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

var (
	namespace       = "nutanix"
	nutanixURL      = flag.String("nutanix.url", "", "Nutanix URL to connect to API https://nutanix.local.host:9440")
	nutanixUser     = flag.String("nutanix.username", "<no value>", "Nutanix API User")
	nutanixPassword = flag.String("nutanix.password", "<no value>", "Nutanix API User Password")
	listenAddress   = flag.String("listen-address", ":9405", "The address to lisiten on for HTTP requests.")
	nutanixConfig   = flag.String("nutanix.conf", "", "Which Nutanixconf.yml file should be used")

	configModTime        time.Time = time.Time{}
	configFileWasMissing           = false
)

type cluster struct {
	Host                string          `yaml:"nutanix_host"`
	Username            string          `yaml:"nutanix_user"`
	Password            string          `yaml:"nutanix_password"`
	LogLevel            string          `yaml:"log_level"`
	MaxParallelRequests int             `yaml:"max_parallel_requests"`
	Collect             map[string]bool `yaml:"collect"`
}

// type clusterCollect struct {
// 	Vms               string `yaml:"vms"`
// 	Cluster           string `yaml:"cluster"`
// 	StorageContainers string `yaml:"storage_containers"`
// 	Hosts             string `yaml:"hosts"`
// }

func main() {
	// add config file watch
	go monitorConfigFileChange()

	// start exporter self-health 30s ticker
	stopTicker := make(chan struct{})
	nutanix.StartHealthTicker(stopTicker)
	defer close(stopTicker)

	flag.Parse()

	//Use locale configfile
	var config map[string]cluster
	var file []byte = nil
	var err error

	if len(*nutanixConfig) > 0 {
		//Read complete Config
		file, err = os.ReadFile(*nutanixConfig)
		if err != nil {
			log.Infof("No config file by name %s found. Using dummy config...", *nutanixConfig)
			file = nil // use default config
			configFileWasMissing = true
		}
	}
	if file == nil {
		file = []byte(fmt.Sprintf("default:\n  nutanix_host: %s\n  nutanix_user: %s\n  nutanix_password: %s}",
			*nutanixURL, *nutanixUser, *nutanixPassword))
	}

	log.Debugf("Config File readed")
	err = yaml.Unmarshal(file, &config)
	if err != nil {
		log.Fatal(err)
	}
	log.Debug("Config file unmarshalled")

	//	http.Handle("/metrics", prometheus.Handler())
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// mark collection boundaries for health per section
		collStart := time.Now()
		// section key might be parsed below; use temporary key first
		sectionKey := r.URL.Query().Get("section")
		if len(sectionKey) == 0 {
			sectionKey = "default"
		}
		nutanix.MarkCollectionStart(sectionKey)
		defer func() { nutanix.MarkCollectionEnd(sectionKey, true, time.Since(collStart)) }()
		params := r.URL.Query()
		section := params.Get("section")
		if len(section) == 0 {
			section = "default"
		}

		// health is always exposed; healthOnly narrows output
		healthOnly := params.Get("health") == "true"

		log.Infof("Section: %s", section)
		log.Debug("Create Nutanix instance")

		var collecthostnics bool = false
		var collectvmnics bool = false
		var maxParallelReq int = 0
		//Write new Parameters (skip section requirement for healthOnly)
		conf, ok := config[section]
		if ok {
			switch strings.ToLower(conf.LogLevel) {
			case "debug":
				log.SetLevel(log.DebugLevel)
			case "trace":
				log.SetLevel(log.TraceLevel)
			default:
				log.SetLevel(log.InfoLevel)
			}
			*nutanixURL = conf.Host
			*nutanixUser = conf.Username
			*nutanixPassword = conf.Password
			maxParallelReq = conf.MaxParallelRequests
			if hostnicsValue, exists := conf.Collect["hostnics"]; exists {
				collecthostnics = hostnicsValue
			}
			if vmnicsValue, exists := conf.Collect["vmnics"]; exists {
				collectvmnics = vmnicsValue
			}
		} else if !healthOnly {
			log.Errorf("Section '%s' not found in config file", section)
			return
		}

		registry := prometheus.NewRegistry()

		// always expose health per section (use host URL as key when available, else section)
		healthSectionKey := section
		if ok && len(conf.Host) > 0 {
			healthSectionKey = conf.Host
		}
		// update sectionKey if conf.Host is available (track by host)
		if ok && len(conf.Host) > 0 {
			sectionKey = conf.Host
		}
		// For health-only requests, use a synthetic UUID; for normal requests, use cluster UUID if available
		healthUUID := "exporter-health"
		if !healthOnly && ok {
			// Try to get cluster UUID from config or use host as identifier
			if len(conf.Host) > 0 {
				healthUUID = conf.Host // Use host as UUID for health metrics
			}
		}
		registry.MustRegister(nutanix.NewExporterHealthCollector(healthSectionKey, healthUUID))
		// If only health is requested, do not touch cluster/API at all
		if healthOnly {
			registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
			registry.MustRegister(prometheus.NewGoCollector())
			h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
			h.ServeHTTP(w, r)
			return
		}

		log.Infof("Host: %s", *nutanixURL)
		nutanixAPI := nutanix.NewNutanix(*nutanixURL, *nutanixUser, *nutanixPassword, maxParallelReq)

		checkCollect := func(c map[string]bool, f string) bool {
			val, exist := c[f]
			return !exist || (exist && val)
		}

		if !healthOnly && checkCollect(config[section].Collect, "storage_containers") {
			log.Debugf("Register StorageContainersCollector")
			registry.MustRegister(nutanix.NewStorageContainersCollector(nutanixAPI))
		}
		if !healthOnly && checkCollect(config[section].Collect, "hosts") {
			log.Debugf("Register HostsCollector")
			registry.MustRegister(nutanix.NewHostsCollector(nutanixAPI, collecthostnics))
		}
		if !healthOnly && checkCollect(config[section].Collect, "cluster") {
			log.Debugf("Register ClusterCollector")
			registry.MustRegister(nutanix.NewClusterCollector(nutanixAPI))
		}
		if !healthOnly && checkCollect(config[section].Collect, "vms") {
			log.Debugf("Register VmsCollector")
			registry.MustRegister(nutanix.NewVmsCollector(nutanixAPI, collectvmnics))
		}
		if !healthOnly && checkCollect(config[section].Collect, "snapshots") {
			log.Debugf("Register Snapshots")
			registry.MustRegister(nutanix.NewSnapshotsCollector(nutanixAPI))
		}
		if !healthOnly && checkCollect(config[section].Collect, "virtual_disks") {
			log.Debugf("Register VirtualDisksCollector")
			registry.MustRegister(nutanix.NewVirtualDisksCollector(nutanixAPI))
		}

		h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
		h.ServeHTTP(w, r)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
		<head><title>Nutanix Exporter</title></head>
		<body>
		<h1>Nutanix Exporter</h1>
		<p><a href="/metrics">Metrics</a></p>
		</body>
		</html>`))
	})

	log.Infof("Starting Server: %s", *listenAddress)
	err = http.ListenAndServe(*listenAddress, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func monitorConfigFileChange() {
	for {
		select {
		case <-time.After(time.Minute):
			fileInfo, err := os.Stat(*nutanixConfig)
			if err != nil {
				log.Errorf("Failed to get config file (%v) err : %v\n", *nutanixConfig, err.Error())
				configFileWasMissing = true
			} else {
				modTime := fileInfo.ModTime()
				if configFileWasMissing || (!configModTime.IsZero() && configModTime != modTime) {
					log.Infof("Config %v file has changed. Restarting exporter...\n", *nutanixConfig)
					os.Exit(1)
				}
				configModTime = modTime
			}
		}
	}
}
