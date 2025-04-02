//
// nutanix-exporter
//
// Prometheus Exportewr for Nutanix API
//
// Author: Martin Weber <martin.weber@de.clara.net>
// Company: Claranet GmbH
//

package nutanix

import (
	"encoding/json"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const KEY_CLUSTER_PROPERTIES = "properties"

// ClusterExporter
type ClusterExporter struct {
	*nutanixExporter
}

// Describe - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *ClusterExporter) Describe(ch chan<- *prometheus.Desc) {

	resp, err := e.api.makeV2Request("GET", "/cluster/")
	if err != nil {
		e.result = nil
		log.Error("Cluster discovery failed")
		return
	}
	data := json.NewDecoder(resp.Body)
	data.Decode(&e.result)

	var stats, usageStats map[string]interface{} = nil, nil

	ent := e.result
	if obj, ok := ent["stats"]; ok {
		stats = obj.(map[string]interface{})
	}
	if obj, ok := ent["usage_stats"]; ok {
		usageStats = obj.(map[string]interface{})
	}

	// Publish cluster properties as separate record
	key := KEY_CLUSTER_PROPERTIES
	e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: e.namespace,
		Name:      key, Help: "..."}, e.properties)
	e.metrics[key].Describe(ch)

	if usageStats != nil {
		for key := range usageStats {
			if _, ok := e.filter_stats[key]; !ok {
				continue
			}
			key = e.normalizeKey(key)

			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid"})

			e.metrics[key].Describe(ch)
		}
	}
	if stats != nil {
		e.addCalculatedStats(stats)
		for key := range stats {
			if _, ok := e.filter_stats[key]; !ok {
				continue
			}

			key = e.normalizeKey(key)

			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid"})

			e.metrics[key].Describe(ch)
		}
	}
	for _, key := range e.fields {
		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key, Help: "..."}, []string{"uuid"})
		e.metrics[key].Describe(ch)
	}
}

func (e *ClusterExporter) addCalculatedStats(stats map[string]interface{}) {
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
}

// Collect - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *ClusterExporter) Collect(ch chan<- prometheus.Metric) {
	// entities, _ := e.result.([]interface{})

	var stats, usageStats map[string]interface{} = nil, nil

	ent := e.result
	if ent == nil {
		return
	}
	if obj, ok := ent["stats"]; ok {
		stats = obj.(map[string]interface{})
	}
	if obj, ok := ent["usage_stats"]; ok {
		usageStats = obj.(map[string]interface{})
	}

	key := KEY_CLUSTER_PROPERTIES
	var property_values []string
	for _, property := range e.properties {
		val := fmt.Sprintf("%v", ent[property])
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

			v := e.valueToFloat64(value)
			// ignore stats which are not available
			if v == -1 {
				continue
			}
			key = e.normalizeKey(key)
			g := e.metrics[key].WithLabelValues(ent["uuid"].(string))
			g.Set(v)
			g.Collect(ch)
		}
	}
	if stats != nil {
		for key, value := range stats {
			if _, ok := e.filter_stats[key]; !ok {
				continue
			}

			v := e.valueToFloat64(value)
			// ignore stats which are not available
			if v == -1 {
				continue
			}
			key = e.normalizeKey(key)
			g := e.metrics[key].WithLabelValues(ent["uuid"].(string))
			g.Set(v)
			g.Collect(ch)
		}
	}
	for _, key := range e.fields {
		log.Debugf("%s > %s", key, ent[key])
		g := e.metrics[key].WithLabelValues(ent["uuid"].(string))
		g.Set(e.valueToFloat64(ent[key]))
		g.Collect(ch)
	}

}

// NewClusterCollector
func NewClusterCollector(_api *Nutanix) *ClusterExporter {

	exporter := &ClusterExporter{
		&nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_cluster",
			fields:     []string{"num_nodes"},
			properties: []string{"uuid", "name", "cluster_external_ipaddress", "version"},
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
			},
		},
	}

	return exporter

}
