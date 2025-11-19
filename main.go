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
	"sync"
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

	configModTime        time.Time    = time.Time{}
	configFileWasMissing              = false
	clusterUUIDCache                  = make(map[string]string) // Cache cluster UUID per section
	clusterUUIDCacheMu   sync.RWMutex                           // Mutex for thread-safe cache access
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

	// Poll cycles are now tracked based on actual collection completions
	// No separate ticker needed - each scrape request from Prometheus receiver
	// represents a poll cycle

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
		// Section is always provided as host IP (e.g., "10.20.10.40") and should match config key
		//Write new Parameters (skip section requirement for healthOnly)
		conf, ok := config[section]
		var healthSectionKey string // Key used for health tracking - must match what nutanix.go uses

		// Health metrics should ALWAYS be collected, regardless of whether section is found or not
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
			// Use host URL as-is for health section key (must match what nutanix.go uses in g.url)
			// nutanix.go uses g.url directly (e.g., "https://10.20.10.40:9440") for health tracking
			// Since section = host IP and conf.Host = full URL, this ensures health tracking matches API calls
			healthSectionKey = conf.Host
			*nutanixUser = conf.Username
			*nutanixPassword = conf.Password
			maxParallelReq = conf.MaxParallelRequests
			if hostnicsValue, exists := conf.Collect["hostnics"]; exists {
				collecthostnics = hostnicsValue
			}
			if vmnicsValue, exists := conf.Collect["vmnics"]; exists {
				collectvmnics = vmnicsValue
			}
		} else {
			// Section not found - still collect health metrics, but use section name as fallback
			// For health-only requests or when section is not found, use section as healthSectionKey
			healthSectionKey = section
			if !healthOnly {
				log.Warnf("Section '%s' not found in config file, but continuing to collect health metrics", section)
			}
		}

		// Use healthSectionKey for all health tracking (must match what nutanix.go uses)
		started := nutanix.MarkCollectionStart(healthSectionKey)
		if !started {
			// Collection already running, return early without tracking end
			// (MarkCollectionEnd should only be called for collections that actually started)
			return
		}

		// Track collection success - starts as true, set to false on errors
		// A collection is considered failed if:
		// - HTTP response writing fails (panic)
		// - Other critical errors occur during collection
		collectionSuccess := true
		defer func() {
			nutanix.MarkCollectionEnd(healthSectionKey, collectionSuccess, time.Since(collStart))
		}()

		registry := prometheus.NewRegistry()

		// Get cluster UUID for health metrics (needed for proper association)
		healthUUID := "exporter-health"  // Default fallback for health-only requests (used as uuid)
		clusterUUID := "exporter-health" // Default fallback for health-only requests (used as cluster_uuid)
		var nutanixAPI *nutanix.Nutanix

		if !healthOnly && ok {
			// Check cache first (thread-safe)
			clusterUUIDCacheMu.RLock()
			cachedUUID, found := clusterUUIDCache[section]
			clusterUUIDCacheMu.RUnlock()

			if found {
				healthUUID = cachedUUID
				clusterUUID = cachedUUID // For cluster-level metrics, cluster_uuid and uuid are the same
				log.Debugf("Using cached cluster UUID for section %s: %s", section, healthUUID)
			} else {
				// Create Nutanix API client and try to get cluster UUID
				log.Infof("Host: %s", *nutanixURL)
				nutanixAPI = nutanix.NewNutanix(*nutanixURL, *nutanixUser, *nutanixPassword, maxParallelReq)
				clusterUUIDValue, err := nutanixAPI.GetClusterUUID()
				if err != nil {
					log.Errorf("Failed to get cluster UUID for health metrics: %v, using section name as fallback", err)
					healthUUID = section // Fallback to section name
					clusterUUID = section
				} else {
					healthUUID = clusterUUIDValue
					clusterUUID = clusterUUIDValue // For cluster-level metrics, cluster_uuid and uuid are the same
					// Cache it for future requests (thread-safe)
					clusterUUIDCacheMu.Lock()
					clusterUUIDCache[section] = clusterUUIDValue
					clusterUUIDCacheMu.Unlock()
					log.Infof("Successfully fetched and cached cluster UUID for section %s: %s", section, healthUUID)
				}
			}
		} else if !healthOnly {
			// Config section not found, use section name as fallback
			healthUUID = section
			clusterUUID = section
		}
		// Use healthSectionKey for collector (must match what nutanix.go uses for tracking)
		// Pass cluster_uuid, uuid, and section to the collector
		registry.MustRegister(nutanix.NewExporterHealthCollector(healthSectionKey, healthUUID, clusterUUID))
		// If only health is requested, do not touch cluster/API at all
		if healthOnly {
			h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
			h.ServeHTTP(w, r)
			return
		}

		// Ensure Nutanix API client is created if not already done
		// Only create API client if we need to collect regular metrics (not health-only)
		if nutanixAPI == nil && !healthOnly {
			if !ok || *nutanixURL == "" {
				// Section not found or missing config - but health metrics are already registered
				// Serve only health metrics and return (this is a successful health-only collection)
				log.Warnf("Cannot create Nutanix API client: missing configuration. Serving only health metrics.")
				h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
				h.ServeHTTP(w, r)
				// collectionSuccess remains true - we successfully served health metrics
				return
			}
			nutanixAPI = nutanix.NewNutanix(*nutanixURL, *nutanixUser, *nutanixPassword, maxParallelReq)
		}

		// Poll cycles are tracked automatically when MarkCollectionEnd is called
		// No ticker needed - each scrape from Prometheus receiver = one poll cycle

		// Only register other collectors if section is found and not health-only
		if !healthOnly && ok {
			checkCollect := func(c map[string]bool, f string) bool {
				val, exist := c[f]
				return !exist || (exist && val)
			}

			if checkCollect(config[section].Collect, "storage_containers") {
				log.Debugf("Register StorageContainersCollector")
				registry.MustRegister(nutanix.NewStorageContainersCollector(nutanixAPI))
			}
			if checkCollect(config[section].Collect, "hosts") {
				log.Debugf("Register HostsCollector")
				registry.MustRegister(nutanix.NewHostsCollector(nutanixAPI, collecthostnics))
			}
			if checkCollect(config[section].Collect, "cluster") {
				log.Debugf("Register ClusterCollector")
				registry.MustRegister(nutanix.NewClusterCollector(nutanixAPI))
			}
			if checkCollect(config[section].Collect, "vms") {
				log.Debugf("Register VmsCollector")
				registry.MustRegister(nutanix.NewVmsCollector(nutanixAPI, collectvmnics))
			}
			if checkCollect(config[section].Collect, "snapshots") {
				log.Debugf("Register Snapshots")
				registry.MustRegister(nutanix.NewSnapshotsCollector(nutanixAPI))
			}
			if checkCollect(config[section].Collect, "virtual_disks") {
				log.Debugf("Register VirtualDisksCollector")
				registry.MustRegister(nutanix.NewVirtualDisksCollector(nutanixAPI))
			}
		}

		h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
		// Track if HTTP response writing fails
		func() {
			defer func() {
				if r := recover(); r != nil {
					collectionSuccess = false
					log.Errorf("Panic while serving metrics: %v", r)
				}
			}()
			h.ServeHTTP(w, r)
		}()
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
