package nutanix

import (
	"encoding/json"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const KEY_HOST_NETWORK_PROPERTIES = "properties"

// HostsNetworkExporter
type HostsNetworkExporter struct {
	*nutanixExporter
	HostUUIDs []string
}

func (e *HostsNetworkExporter) Describe(ch chan<- *prometheus.Desc) {
	log.Info("NewHostsNetworkCollector Describe")
	for _, uuid := range e.HostUUIDs {
		nicEndpoint := fmt.Sprintf("/hosts/%s/host_nics", uuid)
		log.Info("nicEndpoint" + nicEndpoint)
		resp, err := e.api.makeV2Request("GET", nicEndpoint)
		if err != nil {
			log.Errorf("Failed to get host_nics for UUID %s: %v", uuid, err)
			continue
		}

		// Step 4: Decode the /host_nics response
		var hostNicsResult map[string]interface{}
		data := json.NewDecoder(resp.Body)
		if err := data.Decode(&hostNicsResult); err != nil {
			log.Errorf("Failed to decode host_nics response for UUID %s: %v", uuid, err)
			continue
		}

		// Process host_nics information (e.g., store metrics, print, etc.)
		if hostNics, ok := hostNicsResult["entities"].([]interface{}); ok {
			log.Infof("Host %s has %d NIC(s)", uuid, len(hostNics))
			for _, nic := range hostNics {
				nicDetails := nic.(map[string]interface{})
				// Print some nic details for example, you can change this as required
				log.Infof("NIC Details for Host %s: %v", uuid, nicDetails)
			}
		}
	}

}

// Collect - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *HostsNetworkExporter) Collect(ch chan<- prometheus.Metric) {
	log.Info("NewHostsNetworkCollector Collect")
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

		key := KEY_HOST_NETWORK_PROPERTIES
		var property_values []string
		for _, property := range e.properties {
			val := fmt.Sprintf("%v", ent[property])
			property_values = append(property_values, val)
		}
		//log.Info(e.HostUUIDs)
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
			log.Debugf("%s > %s", key, ent[key])
			g := e.metrics[key].WithLabelValues(ent["uuid"].(string), ent["cluster_uuid"].(string))
			g.Set(e.valueToFloat64(ent[key]))
			g.Collect(ch)
		}
	}
}

// NewHostsNetworkCollector
func NewHostsNetworkCollector(_api *Nutanix) *HostsNetworkExporter {
	log.Info("NewHostsNetworkCollector call")
	return &HostsNetworkExporter{
		nutanixExporter: &nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_hosts_network",
			properties: []string{"node_uuid", "uuid", "name", "mac_address", "ipv4_addresses", "name", "mtu_in_bytes"},
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
