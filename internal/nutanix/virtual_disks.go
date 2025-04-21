package nutanix

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const (
	KEY_VIRTUAL_DISK_PROPERTIES = "properties"
	METRIC_TOTAL_USAGE_BYTES    = "controller.storage_tier.total.usage_bytes"
	METRIC_TOTAL_WRITE_IO_SIZE  = "controller_total_write_io_size_kbytes"
)

type VirtualDisksExporter struct {
	*nutanixExporter
}

func (e *VirtualDisksExporter) Describe(ch chan<- *prometheus.Desc) {
	resp, err := e.api.makeV2Request("GET", "/virtual_disks/")
	if err != nil {
		e.result = nil
		log.Error("Virtual disk discovery failed")
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
		var stats map[string]interface{} = nil

		ent := entity.(map[string]interface{})
		if obj, ok := ent["stats"]; ok {
			stats = obj.(map[string]interface{})
		}

		// Publish host properties as separate record
		key := KEY_HOST_PROPERTIES
		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key, Help: "..."}, e.properties)
		e.metrics[key].Describe(ch)

		if stats != nil {
			e.addCalculatedStats(stats)
			for key := range stats {
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}

				key = e.normalizeKey(key)

				e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: e.namespace,
					Name:      key, Help: "..."}, []string{"uuid", "attached_vm_uuid"})

				e.metrics[key].Describe(ch)
			}
		}
		for _, key := range e.fields {
			e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      key, Help: "..."}, []string{"uuid", "attached_vm_uuid"})
			e.metrics[key].Describe(ch)
		}

	}

}

func (e *VirtualDisksExporter) addCalculatedStats(stats map[string]interface{}) {
	if stats == nil {
		return
	}
	// Add usage stats
	usage_bytes_metric_keys := []string{
		"controller.storage_tier.cloud.pinned_usage_bytes",
		"controller.storage_tier.cloud.usage_bytes",
		"controller.storage_tier.das-sata.pinned_usage_bytes",
		"controller.storage_tier.das-sata.usage_bytes",
		"controller.storage_tier.ssd.pinned_usage_bytes",
		"controller.storage_tier.ssd.usage_bytes",
	}
	var usage_bytes float64 = 0
	for _, key := range usage_bytes_metric_keys {
		val, ok := stats[key]
		if ok {
			v := e.valueToFloat64(val)
			if v > 0 {
				usage_bytes = usage_bytes + e.valueToFloat64(val)
			}
		}
	}
	stats[METRIC_TOTAL_USAGE_BYTES] = usage_bytes

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

func (e *VirtualDisksExporter) Collect(ch chan<- prometheus.Metric) {
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

		key := KEY_HOST_PROPERTIES
		var property_values []string
		for _, property := range e.properties {
			var val string = ""
			// format properties
			switch property {
			case "disk_capacity_in_mb":
				propname := "disk_capacity_in_bytes"
				obj := ent[propname]
				if obj != nil {
					floatval := e.valueToFloat64(obj)
					floatval = floatval / (1024 * 1024)
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

		val := ent["attached_vm_uuid"]
		var vmUUID string = ""
		if val != nil {
			vmUUID = val.(string)
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
				// ignore histogram stats
				if strings.Contains(key, "histogram") {
					continue
				}
				key = e.normalizeKey(key)
				g := e.metrics[key].WithLabelValues(ent["uuid"].(string), vmUUID)
				g.Set(val)
				g.Collect(ch)
			}

		}
		for _, key := range e.fields {
			g := e.metrics[key].WithLabelValues(ent["uuid"].(string), vmUUID)
			g.Set(e.valueToFloat64(ent[key]))
			g.Collect(ch)
		}
		log.Debug("Virtual Disk data collected")
	}
}

func NewVirtualDisksCollector(_api *Nutanix) *VirtualDisksExporter {

	return &VirtualDisksExporter{
		&nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_vdisks",
			fields:     []string{"disk_capacity_in_bytes"},
			properties: []string{"uuid", "attached_vm_uuid", "attached_vmname", "storage_container_uuid", "cluster_uuid", "disk_address", "disk_capacity_in_mb"},
			filter_stats: map[string]bool{
				"controller_total_read_io_size_kbytes":  true,
				"controller_total_io_size_kbytes":       true,
				"controller_num_read_io":                true,
				"controller_num_write_io":               true,
				"controller_avg_read_io_latency_usecs":  true,
				"controller_avg_write_io_latency_usecs": true,
				//usage stats
				"controller.storage_tier.cloud.pinned_usage_bytes":    true,
				"controller.storage_tier.cloud.usage_bytes":           true,
				"controller.storage_tier.das-sata.pinned_usage_bytes": true,
				"controller.storage_tier.das-sata.usage_bytes":        true,
				"controller.storage_tier.ssd.pinned_usage_bytes":      true,
				"controller.storage_tier.ssd.usage_bytes":             true,
				// Calculated
				METRIC_TOTAL_WRITE_IO_SIZE: true,
				METRIC_TOTAL_USAGE_BYTES:   true,
			},
		},
	}
}
