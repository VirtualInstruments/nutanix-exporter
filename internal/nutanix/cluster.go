// nutanix-exporter
//
// # Prometheus Exportewr for Nutanix API
//
// Author: Martin Weber <martin.weber@de.clara.net>
// Company: Claranet GmbH

package nutanix

import (
	"encoding/json"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const KEY_CLUSTER_PROPERTIES = "properties"

type ClusterExporter struct {
	*nutanixExporter
}

func (e *nutanixExporter) hasAllProperties(ent map[string]interface{}) bool {
	for _, prop := range e.properties {
		if val, ok := ent[prop]; !ok || val == nil {
			return false
		}
	}
	return true
}

func (e *nutanixExporter) getLabelValues(ent map[string]interface{}) []string {
	var values []string
	for _, prop := range e.properties {
		values = append(values, fmt.Sprintf("%v", ent[prop]))
	}
	return values
}

func (e *ClusterExporter) Describe(ch chan<- *prometheus.Desc) {

	resp, err := e.api.makeV2Request("GET", "/cluster/")
	if err != nil {
		e.result = nil
		log.Error("Cluster discovery failed")
		return
	}
	defer resp.Body.Close()

	data := json.NewDecoder(resp.Body)
	data.Decode(&e.result)
	ent := e.result

	if !e.hasAllProperties(ent) {
		log.Warn("Skipping Describe: cluster object missing properties")
		return
	}

	e.metrics[KEY_CLUSTER_PROPERTIES] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: e.namespace,
		Name:      KEY_CLUSTER_PROPERTIES,
		Help:      "Cluster properties metric",
	}, e.properties)
	e.metrics[KEY_CLUSTER_PROPERTIES].Describe(ch)

	// Extract usage_stats and stats
	var usageStats, stats map[string]interface{}
	if obj, ok := ent["usage_stats"]; ok {
		usageStats = obj.(map[string]interface{})
	}
	if obj, ok := ent["stats"]; ok {
		stats = obj.(map[string]interface{})
	}

	// Add calculated stats
	if stats != nil {
		e.addCalculatedStats(stats)
	}

	// Describe usage_stats
	for key := range usageStats {
		if !e.filter_stats[key] {
			continue
		}
		nKey := e.normalizeKey(key)
		e.metrics[nKey] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      nKey,
			Help:      "Cluster usage stat",
		}, e.properties)
		e.metrics[nKey].Describe(ch)
	}

	// Describe stats
	for key := range stats {
		if !e.filter_stats[key] {
			continue
		}
		nKey := e.normalizeKey(key)
		e.metrics[nKey] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      nKey,
			Help:      "Cluster stat",
		}, e.properties)
		e.metrics[nKey].Describe(ch)
	}

	// Describe fields
	for _, key := range e.fields {
		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key,
			Help:      "Cluster field",
		}, e.properties)
		e.metrics[key].Describe(ch)
	}
}

// Collect - implements prometheus.Collector
func (e *ClusterExporter) Collect(ch chan<- prometheus.Metric) {
	ent := e.result
	if ent == nil || !e.hasAllProperties(ent) {
		log.Warn("Skipping Collect: cluster object missing or incomplete")
		return
	}
	labelValues := e.getLabelValues(ent)

	// Set cluster properties metric
	g := e.metrics[KEY_CLUSTER_PROPERTIES].WithLabelValues(labelValues...)
	g.Set(1)
	g.Collect(ch)

	// usage_stats
	if usageStats, ok := ent["usage_stats"].(map[string]interface{}); ok {
		for key, value := range usageStats {
			if !e.filter_stats[key] {
				continue
			}
			v := e.valueToFloat64(value)
			if v == -1 {
				continue
			}
			nKey := e.normalizeKey(key)
			g := e.metrics[nKey].WithLabelValues(labelValues...)
			g.Set(v)
			g.Collect(ch)
		}
	}

	// stats
	if stats, ok := ent["stats"].(map[string]interface{}); ok {
		e.addCalculatedStats(stats)
		for key, value := range stats {
			if !e.filter_stats[key] {
				continue
			}
			v := e.valueToFloat64(value)
			if v == -1 {
				continue
			}
			nKey := e.normalizeKey(key)
			g := e.metrics[nKey].WithLabelValues(labelValues...)
			g.Set(v)
			g.Collect(ch)
		}
	}

	// fields
	for _, key := range e.fields {
		v := e.valueToFloat64(ent[key])
		g := e.metrics[key].WithLabelValues(labelValues...)
		g.Set(v)
		g.Collect(ch)
	}

	log.Debug("Cluster data collected for UUID: ", ent["uuid"].(string))
}

// addCalculatedStats adds derived metrics to stats
func (e *ClusterExporter) addCalculatedStats(stats map[string]interface{}) {
	if stats == nil {
		return
	}

	var totalSize, readSize float64
	if val, ok := stats["controller_total_io_size_kbytes"]; ok {
		totalSize = e.valueToFloat64(val)
	}
	if val, ok := stats["controller_total_read_io_size_kbytes"]; ok {
		readSize = e.valueToFloat64(val)
	}
	stats[METRIC_TOTAL_WRITE_IO_SIZE] = totalSize - readSize
}

// NewClusterCollector creates the cluster exporter
func NewClusterCollector(_api *Nutanix) *ClusterExporter {
	return &ClusterExporter{
		&nutanixExporter{
			api:       *_api,
			metrics:   make(map[string]*prometheus.GaugeVec),
			namespace: "nutanix_cluster",
			fields:    []string{"num_nodes"},
			properties: []string{
				"uuid", "name", "cluster_external_ipaddress", "version",
			},
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
				METRIC_TOTAL_WRITE_IO_SIZE:              true,
			},
		},
	}
}
