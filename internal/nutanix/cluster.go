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

	resp, _ := e.api.makeRequest("GET", "/cluster/")
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
			key = e.normalizeKey(key)

			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid"})

			e.metrics[key].Describe(ch)
		}
	}
	if stats != nil {
		for key := range stats {
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

// Collect - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *ClusterExporter) Collect(ch chan<- prometheus.Metric) {
	// entities, _ := e.result.([]interface{})

	var stats, usageStats map[string]interface{} = nil, nil

	ent := e.result
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
			key = e.normalizeKey(key)

			v := e.valueToFloat64(value)

			g := e.metrics[key].WithLabelValues(ent["uuid"].(string))
			g.Set(v)
			g.Collect(ch)
		}
	}
	if stats != nil {
		for key, value := range stats {
			key = e.normalizeKey(key)

			v := e.valueToFloat64(value)

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
		}}

	return exporter

}
