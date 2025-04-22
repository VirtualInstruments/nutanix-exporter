package nutanix

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const KEY_VM_NIC_PROPERTIES = "properties"

// VMNicsExporter
type VMNicsExporter struct {
	*nutanixExporter
	VMUUID string
	VMName string
}

func (e *VMNicsExporter) Describe(ch chan<- *prometheus.Desc) {
	uuid := e.VMUUID

	// Construct the NIC endpoint using the single vm UUID
	nicEndpoint := fmt.Sprintf("/vms/%s/virtual_nics", uuid)
	log.Debug("VM Nic Endpoint: " + nicEndpoint)

	// Make the API request to fetch vm NICs information
	resp, err := e.api.makeV1Request("GET", nicEndpoint)
	if err != nil {
		e.result = nil
		log.Error("VM nic discovery failed")
		return
	}

	var entitiesArray []any = make([]any, 0)

	data := json.NewDecoder(resp.Body)
	data.Decode(&entitiesArray)

	var entities []interface{} = nil
	if len(entitiesArray) > 0 {
		entities = entitiesArray
		e.result = map[string]interface{}{
			"entities": entities,
		}
	}
	if entities == nil {
		return
	}

	for _, entity := range entities {

		var stats map[string]interface{} = nil

		ent := entity.(map[string]interface{})
		if obj, ok := ent["stats"]; ok {
			stats = obj.(map[string]interface{})
		}

		// Publish vm properties as separate record
		key := KEY_VM_NIC_PROPERTIES
		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key, Help: "..."}, e.properties)
		e.metrics[key].Describe(ch)

		if stats != nil {
			for key := range stats {
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}

				key = e.normalizeKey(key)
				e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: e.namespace,
					Name:      key, Help: "..."}, []string{"uuid", "vmUuid"})

				e.metrics[key].Describe(ch)
			}
		}
	}

}

// Collect - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *VMNicsExporter) Collect(ch chan<- prometheus.Metric) {
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
		var stats map[string]interface{} = nil

		ent := entity.(map[string]interface{})
		if obj, ok := ent["stats"]; ok {
			stats = obj.(map[string]interface{})
		}

		key := KEY_VM_NIC_PROPERTIES
		var property_values []string
		for _, property := range e.properties {
			var val string = ""
			// format properties
			switch property {
			case "ipv4Addresses":
				obj := ent[property]
				if obj != nil {
					strarr := []string{}
					for _, addr := range obj.([]interface{}) {
						strarr = append(strarr, addr.(string))
					}
					val = strings.Join(strarr, ",")
				}
			case "vmName":
				val = e.VMName
			default:
				obj := ent[property]
				if obj != nil {
					val = fmt.Sprintf("%v", ent[property])
				}
			}
			property_values = append(property_values, val)
		}
		g := e.metrics[key].WithLabelValues(property_values...)
		g.Set(1)
		g.Collect(ch)

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
				g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["vmUuid"].(string))
				g.Set(val)
				g.Collect(ch)
			}
		}
		for _, key := range e.fields {
			g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["vmUuid"].(string))
			g.Set(e.valueToFloat64(ent[key]))
			g.Collect(ch)
		}
		log.Debugf("VMs NIC data collected for VM=%s VM_UUID=%s", e.VMName, e.VMUUID)
	}
}

// NewVMsNetworkCollector
func NewVMsNetworkCollector(_api *Nutanix, vmname string, vmuuid string) *VMNicsExporter {
	return &VMNicsExporter{
		VMName: vmname,
		VMUUID: vmuuid,
		nutanixExporter: &nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_vmnics",
			properties: []string{"vmUuid", "uuid", "vmName", "macAddress", "ipv4Addresses", "name", "mtuInBytes"},
			filter_stats: map[string]bool{
				"network.received_bytes":         true,
				"network.received_pkts":          true,
				"network.error_received_pkts":    true,
				"network.transmitted_bytes":      true,
				"network.transmitted_pkts":       true,
				"network.error_transmitted_pkts": true,
			},
		},
	}
}
