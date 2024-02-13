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

const KEY_HOST_PROPERTIES = "properties"

// HostsExporter
type HostsExporter struct {
	*nutanixExporter
}

// Describe - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *HostsExporter) Describe(ch chan<- *prometheus.Desc) {
	resp, _ := e.api.makeRequest("GET", "/hosts/")
	data := json.NewDecoder(resp.Body)

	data.Decode(&e.result)
	entities, _ := e.result["entities"].([]interface{})

	for _, entity := range entities {
		ent := entity.(map[string]interface{})
		stats := ent["stats"].(map[string]interface{})
		usageStats := ent["usage_stats"].(map[string]interface{})

		// Publish host properties as separate record
		key := KEY_HOST_PROPERTIES
		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key, Help: "..."}, e.properties)
		e.metrics[key].Describe(ch)

		for key := range usageStats {
			key = e.normalizeKey(key)

			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid", "cluster_uuid"})

			e.metrics[key].Describe(ch)
		}

		for key := range stats {
			key = e.normalizeKey(key)

			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid", "cluster_uuid"})

			e.metrics[key].Describe(ch)
		}

		for _, key := range e.fields {
			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid", "cluster_uuid"})
			e.metrics[key].Describe(ch)
		}

	}

}

// Collect - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *HostsExporter) Collect(ch chan<- prometheus.Metric) {

	entities, _ := e.result["entities"].([]interface{})

	for _, entity := range entities {
		ent := entity.(map[string]interface{})
		stats := ent["stats"].(map[string]interface{})
		usageStats := ent["usage_stats"].(map[string]interface{})

		key := KEY_HOST_PROPERTIES
		var property_values []string
		for _, property := range e.properties {
			val := fmt.Sprintf("%v", ent[property])
			property_values = append(property_values, val)
		}
		g := e.metrics[key].WithLabelValues(property_values...)
		g.Set(1)
		g.Collect(ch)

		for key, value := range usageStats {
			key = e.normalizeKey(key)

			g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["cluster_uuid"].(string))
			g.Set(e.valueToFloat64(value))
			g.Collect(ch)
		}
		for key, value := range stats {
			key = e.normalizeKey(key)

			g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["cluster_uuid"].(string))
			g.Set(e.valueToFloat64(value))
			g.Collect(ch)
		}

		for _, key := range e.fields {
			log.Debugf("%s > %s", key, ent[key])
			g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["cluster_uuid"].(string))
			g.Set(e.valueToFloat64(ent[key]))
			g.Collect(ch)
		}
	}
}

// NewHostsCollector
func NewHostsCollector(_api *Nutanix) *HostsExporter {

	return &HostsExporter{
		&nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_hosts",
			fields:     []string{"num_vms", "num_cpu_cores", "num_cpu_sockets", "num_cpu_threads", "cpu_frequency_in_hz", "cpu_capacity_in_hz", "memory_capacity_in_bytes", "boot_time_in_usecs"},
			properties: []string{"uuid", "cluster_uuid", "name", "host_type", "hypervisor_address", "serial"},
		}}
}
