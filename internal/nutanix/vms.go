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

const KEY_VM_PROPERTIES = "properties"

// VmsExporter
type VmsExporter struct {
	*nutanixExporter
}

// Describe - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *VmsExporter) Describe(ch chan<- *prometheus.Desc) {
	resp, _ := e.api.makeRequest("GET", "/vms/")
	data := json.NewDecoder(resp.Body)
	data.Decode(&e.result)

	// Publish VM properties as separate record
	key := KEY_VM_PROPERTIES
	e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: e.namespace,
		Name:      key, Help: "..."}, e.properties)
	e.metrics[key].Describe(ch)

	var metadata map[string]interface{} = nil
	if obj, ok := e.result["metadata"]; ok {
		metadata = obj.(map[string]interface{})
	}

	if metadata != nil {
		for key := range metadata {
			key = e.normalizeKey(key)
			log.Debugf("Register Key %s", key)

			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{})

			e.metrics[key].Describe(ch)
		}
	}
	for _, key := range e.fields {
		key = e.normalizeKey(key)

		log.Debugf("Register Key %s", key)

		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key, Help: "..."}, []string{"uuid", "host_uuid"})

		e.metrics[key].Describe(ch)
	}

}

// Collect - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *VmsExporter) Collect(ch chan<- prometheus.Metric) {

	var metadata map[string]interface{} = nil
	if obj, ok := e.result["metadata"]; ok {
		metadata = obj.(map[string]interface{})
	}

	if metadata != nil {
		for key, value := range metadata {
			key = e.normalizeKey(key)
			log.Debugf("Collect Key %s", key)

			g := e.metrics[key].WithLabelValues()
			g.Set(e.valueToFloat64(value))
			g.Collect(ch)
		}
	}
	var key string
	var g prometheus.Gauge

	var entities []interface{} = nil
	if obj, ok := e.result["entities"]; ok {
		entities = obj.([]interface{})
	}
	if entities == nil {
		return
	}

	for _, entity := range entities {
		var ent = entity.(map[string]interface{})

		key = KEY_VM_PROPERTIES
		var property_values []string
		for _, property := range e.properties {
			val := fmt.Sprintf("%v", ent[property])
			property_values = append(property_values, val)
		}
		g = e.metrics[key].WithLabelValues(property_values...)
		g.Set(1)
		g.Collect(ch)

		for _, key := range e.fields {
			key = e.normalizeKey(key)
			log.Debugf("Collect Key %s", key)

			val := ent["host_uuid"]
			var hostUUID string = ""
			if val != nil {
				hostUUID = val.(string)
			}
			g = e.metrics[key].WithLabelValues(ent["uuid"].(string), hostUUID)

			if key == "power_state" {
				if ent[key] == "on" {
					g.Set(1)
				} else {
					g.Set(0)
				}
			} else {
				g.Set(e.valueToFloat64(ent[key]))
			}

			g.Collect(ch)
		}
	}

}

// NewVmsCollector - Create the Collector for VMs
func NewVmsCollector(_api *Nutanix) *VmsExporter {

	return &VmsExporter{
		&nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_vms",
			fields:     []string{"num_cores_per_vcpu", "memory_mb", "num_vcpus", "power_state", "vcpu_reservation_hz"},
			properties: []string{"uuid", "host_uuid", "name", "memory_mb", "num_vcpus", "power_state"},
		}}
}
