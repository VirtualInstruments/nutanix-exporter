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
	resp, _ := e.api.makeV1Request("GET", "/vms/")
	data := json.NewDecoder(resp.Body)
	data.Decode(&e.result)

	var entities []interface{} = nil
	if obj, ok := e.result["entities"]; ok {
		entities = obj.([]interface{})
	}
	if entities == nil || len(entities) == 0 {
		return
	}

	// Publish VM properties as separate record
	key := KEY_VM_PROPERTIES
	property_keys := []string{}
	for _, key := range e.properties {
		// Renaming keys
		switch key {
		case "hostUuid":
			key = "host_uuid"
		}
		property_keys = append(property_keys, key)
	}
	e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: e.namespace,
		Name:      key, Help: "..."}, property_keys)
	e.metrics[key].Describe(ch)

	ent := entities[0].(map[string]interface{})
	var stats map[string]interface{} = nil
	if obj, ok := ent["stats"]; ok {
		stats = obj.(map[string]interface{})
	}
	if stats != nil {
		for key := range stats {
			key = e.normalizeKey(key)

			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid", "host_uuid"})

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

	var key string
	var g prometheus.Gauge

	var entities []interface{} = nil
	if obj, ok := e.result["entities"]; ok {
		entities = obj.([]interface{})
	}
	if entities == nil || len(entities) == 0 {
		return
	}

	for _, entity := range entities {
		var ent = entity.(map[string]interface{})

		var stats map[string]interface{} = nil
		if obj, ok := ent["stats"]; ok {
			stats = obj.(map[string]interface{})
		}

		key = KEY_VM_PROPERTIES
		var property_values []string
		for _, property := range e.properties {
			val := fmt.Sprintf("%v", ent[property])
			property_values = append(property_values, val)
		}
		g = e.metrics[key].WithLabelValues(property_values...)
		g.Set(1)
		g.Collect(ch)

		val := ent["hostUuid"]
		var hostUUID string = ""
		if val != nil {
			hostUUID = val.(string)
		}

		if stats != nil {
			for key, value := range stats {
				val := e.valueToFloat64(value)
				// ignore stats which are not available
				if val == -1 {
					continue
				}
				key = e.normalizeKey(key)
				g := e.metrics[key].WithLabelValues(ent["uuid"].(string), hostUUID)
				g.Set(val)
				g.Collect(ch)
			}
		}

		for _, key := range e.fields {
			key = e.normalizeKey(key)
			log.Debugf("Collect Key %s", key)

			g = e.metrics[key].WithLabelValues(ent["uuid"].(string), hostUUID)

			if key == "powerState" {
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
			fields:     []string{"memoryCapacityInBytes", "numVCpus", "powerState", "cpuReservedInHz"},
			properties: []string{"uuid", "hostUuid", "vmName", "memoryCapacityInBytes", "numVCpus", "powerState"},
		}}
}
