//
// nutanix-exporter
//
// Prometheus Exporter for Nutanix API
//
// Author: Martin Weber <martin.weber@de.clara.net>
// Company: Claranet GmbH
//

package nutanix

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const KEY_HOST_PROPERTIES = "properties"

// HostsExporter
type HostsExporter struct {
	*nutanixExporter
	networkExporters map[string]*HostNicsExporter
	collecthostnics  bool
}

// Describe - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *HostsExporter) Describe(ch chan<- *prometheus.Desc) {
	resp, err := e.api.makeV2Request("GET", "/hosts/")
	if err != nil {
		e.result = nil
		log.Error("Host discovery failed")
		return
	}

	data := json.NewDecoder(resp.Body)
	data.Decode(&e.result)

	var entities []interface{} = nil
	if obj, ok := e.result["entities"]; ok {
		entities = obj.([]interface{})
	}
	if entities == nil {
		return
	}

	for _, entity := range entities {
		var stats, usageStats map[string]interface{} = nil, nil

		ent := entity.(map[string]interface{})
		if obj, ok := ent["stats"]; ok {
			stats = obj.(map[string]interface{})
		}
		if obj, ok := ent["usage_stats"]; ok {
			usageStats = obj.(map[string]interface{})
		}

		// Publish host properties as separate record
		key := KEY_HOST_PROPERTIES
		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key, Help: "..."}, e.properties)
		e.metrics[key].Describe(ch)

		if e.collecthostnics {
			var hostName string
			if obj, ok := ent["name"]; ok {
				hostName = obj.(string)
			}
			if obj, ok := ent["uuid"]; ok {
				uuid := obj.(string)
				e.networkExporters[uuid] = NewHostsNetworkCollector(&e.api, hostName, uuid)
			}
		}

		if usageStats != nil {
			for key := range usageStats {
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}

				key = e.normalizeKey(key)

				e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: e.namespace,
					Name:      key, Help: "..."}, []string{"uuid", "cluster_uuid"})

				e.metrics[key].Describe(ch)
			}
		}
		if stats != nil {
			e.addCalculatedStats(ent, stats)
			for key := range stats {
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}

				key = e.normalizeKey(key)
				e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: e.namespace,
					Name:      key, Help: "..."}, []string{"uuid", "cluster_uuid"})

				e.metrics[key].Describe(ch)
			}
		}
		for _, key := range e.fields {
			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid", "cluster_uuid"})
			e.metrics[key].Describe(ch)
		}

	}

	e.DescribeNicsParallel(ch)
}

func (e *HostsExporter) addCalculatedStats(ent map[string]interface{}, stats map[string]interface{}) {
	if stats == nil {
		return
	}

	// Calculate write io size
	var total_size, read_size float64 = 0, 0
	val, ok := stats["controller_total_io_size_kbytes"]
	if ok {
		v := e.valueToFloat64(val)
		if v > 0 {
			total_size = v
		}
	}
	val, ok = stats["controller_total_read_io_size_kbytes"]
	if ok {
		v := e.valueToFloat64(val)
		if v > 0 {
			read_size = v
		}
	}
	stats[METRIC_TOTAL_WRITE_IO_SIZE] = total_size - read_size

	// Add free memory stat
	mem_total := e.valueToFloat64(ent["memory_capacity_in_bytes"])
	var mem_usage_ppm float64 = 0
	val, ok = stats["hypervisor_memory_usage_ppm"]
	if ok {
		v := e.valueToFloat64(val)
		if v > 0 {
			mem_usage_ppm = v
		}
	}
	mem_used := (mem_usage_ppm / 1000000) * mem_total
	stats[METRIC_MEM_USAGE_BYTES] = mem_used
	stats[METRIC_MEM_FREE_BYTES] = mem_total - mem_used
}

// Collect - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *HostsExporter) Collect(ch chan<- prometheus.Metric) {
	if e.result == nil {
		return
	}
	var entities []interface{} = nil
	if obj, ok := e.result["entities"]; ok {
		entities = obj.([]interface{})
	}
	if entities == nil {
		return
	}

	for _, entity := range entities {
		var stats, usageStats map[string]interface{} = nil, nil

		ent := entity.(map[string]interface{})
		if obj, ok := ent["stats"]; ok {
			stats = obj.(map[string]interface{})
		}
		if obj, ok := ent["usage_stats"]; ok {
			usageStats = obj.(map[string]interface{})
		}

		key := KEY_HOST_PROPERTIES
		var property_values []string

		for _, property := range e.properties {
			var val string = ""
			// format properties
			switch property {
			case "memory_capacity_in_mb":
				propname := strings.Replace(property, "_mb", "_bytes", 1)
				obj := ent[propname]
				if obj != nil {
					floatval := e.valueToFloat64(obj)
					floatval = floatval / (1024 * 1024)
					val = strconv.FormatFloat(floatval, 'f', 0, 64)
				}
			case "cpu_frequency_in_mhz", "cpu_capacity_in_mhz":
				propname := strings.Replace(property, "mhz", "hz", 1)
				obj := ent[propname]
				if obj != nil {
					floatval := e.valueToFloat64(obj)
					floatval = floatval / 1000000
					val = strconv.FormatFloat(floatval, 'f', 0, 64)
				}
			case "num_vms", "num_cpu_cores", "num_cpu_sockets", "num_cpu_threads":
				obj := ent[property]
				if obj != nil {
					floatval := e.valueToFloat64(obj)
					val = strconv.FormatFloat(floatval, 'f', 0, 64)
				}
			default:
				obj := ent[property]
				if obj != nil {
					val = ent[property].(string)
				}
			}
			property_values = append(property_values, val)
		}
		g := e.metrics[key].WithLabelValues(property_values...)
		g.Set(1)
		g.Collect(ch)

		if usageStats != nil {
			for key, value := range usageStats {
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}

				val := e.valueToFloat64(value)
				// ignore stats which are not available
				if val == -1 {
					continue
				}
				key = e.normalizeKey(key)
				g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["cluster_uuid"].(string))
				g.Set(val)
				g.Collect(ch)
			}
		}
		if stats != nil {
			for key, value := range stats {
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}

				val := e.valueToFloat64(value)
				// ignore stats which are not available
				if val == -1 {
					continue
				}
				key = e.normalizeKey(key)
				g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["cluster_uuid"].(string))
				g.Set(val)
				g.Collect(ch)
			}
		}
		for _, key := range e.fields {
			g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["cluster_uuid"].(string))
			g.Set(e.valueToFloat64(ent[key]))
			g.Collect(ch)
		}
		log.Debugf("Host data collected for host: UUID=%s, Name=%s", ent["uuid"], ent["name"])
	}

	for hostUUID, networkExporter := range e.networkExporters {
		log.Debugf("Collect nic metrics for host UUID: %s", hostUUID)
		networkExporter.Collect(ch)
	}
}

// NewHostsCollector
func NewHostsCollector(_api *Nutanix, collecthostnics bool) *HostsExporter {
	return &HostsExporter{
		networkExporters: make(map[string]*HostNicsExporter),
		collecthostnics:  collecthostnics,
		nutanixExporter: &nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_hosts",
			fields:     []string{"num_vms", "num_cpu_cores", "num_cpu_sockets", "num_cpu_threads", "cpu_frequency_in_hz", "cpu_capacity_in_hz", "memory_capacity_in_bytes", "boot_time_in_usecs"},
			properties: []string{"uuid", "cluster_uuid", "name", "host_type", "hypervisor_address", "serial", "hypervisor_full_name", "num_vms", "num_cpu_cores", "num_cpu_sockets", "num_cpu_threads", "cpu_frequency_in_mhz", "cpu_capacity_in_mhz", "memory_capacity_in_mb", "block_model_name"},
			filter_stats: map[string]bool{
				"storage.capacity_bytes":                true,
				"storage.usage_bytes":                   true,
				"storage.logical_usage_bytes":           true,
				"controller_total_read_io_size_kbytes":  true,
				"controller_total_io_size_kbytes":       true,
				"controller_num_read_io":                true,
				"controller_num_write_io":               true,
				"controller_avg_read_io_latency_usecs":  true,
				"controller_avg_write_io_latency_usecs": true,
				"hypervisor_cpu_usage_ppm":              true,
				"cpu_capacity_in_hz":                    true,
				"hypervisor_memory_usage_ppm":           true,
				"hypervisor_num_received_bytes":         true,
				"hypervisor_num_transmitted_bytes":      true,
				// Calculated
				METRIC_TOTAL_WRITE_IO_SIZE: true,
				METRIC_MEM_USAGE_BYTES:     true,
				METRIC_MEM_FREE_BYTES:      true,
			},
		},
	}
}

func (e *HostsExporter) DescribeNicsParallel(ch chan<- *prometheus.Desc) {
	var wg sync.WaitGroup
	// Create a buffered channel to limit concurrent Describe calls
	semaphore := make(chan struct{}, e.api.maxParallelRequests)
	for hostUUID, networkExporter := range e.networkExporters {
		wg.Add(1)
		go func(hostUUID string, exporter *HostNicsExporter) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire a token
			defer func() { <-semaphore }() // Release the token
			log.Debugf("Describing host nic metrics for host UUID: %s", hostUUID)
			exporter.Describe(ch)
		}(hostUUID, networkExporter)
	}
	wg.Wait()
}
