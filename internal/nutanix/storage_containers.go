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
)

const KEY_STORAGE_CONTAINER_PROPERTIES = "properties"

type StorageContainerExporter struct {
	*nutanixExporter
}

// Describe - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *StorageContainerExporter) Describe(ch chan<- *prometheus.Desc) {
	// prometheus.DescribeByCollect(e, ch)

	resp, _ := e.api.makeV2Request("GET", "/storage_containers/")
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
		key := KEY_STORAGE_CONTAINER_PROPERTIES
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
					Name:      key, Help: "..."}, []string{"storage_container_uuid", "cluster_uuid"})

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
					Name:      key, Help: "..."}, []string{"storage_container_uuid", "cluster_uuid"})

				e.metrics[key].Describe(ch)
			}
		}
	}
}

func (e *StorageContainerExporter) addCalculatedStats(stats map[string]interface{}) {
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
func (e *StorageContainerExporter) Collect(ch chan<- prometheus.Metric) {
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

		key := KEY_STORAGE_CONTAINER_PROPERTIES
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

				val := e.valueToFloat64(value)
				// ignore stats which are not available
				if val == -1 {
					continue
				}
				key = e.normalizeKey(key)
				g := e.metrics[key].WithLabelValues(ent["storage_container_uuid"].(string), ent["cluster_uuid"].(string))
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
				g := e.metrics[key].WithLabelValues(ent["storage_container_uuid"].(string), ent["cluster_uuid"].(string))
				g.Set(val)
				g.Collect(ch)
			}
		}
	}
}

// NewStorageContainersCollector
func NewStorageContainersCollector(_api *Nutanix) *StorageContainerExporter {

	return &StorageContainerExporter{
		&nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_storage_containers",
			properties: []string{"storage_container_uuid", "cluster_uuid", "name", "replication_factor", "compression_enabled", "max_capacity"},
			filter_stats: map[string]bool{
				"storage.usage_bytes":                       true,
				"storage.capacity_bytes":                    true,
				"storage.logical_usage_bytes":               true,
				"storage.container_reserved_capacity_bytes": true,
				"controller_total_read_io_size_kbytes":      true,
				"controller_total_io_size_kbytes":           true,
				"controller_num_read_iops":                  true,
				"controller_num_write_iops":                 true,
				"controller_avg_read_io_latency_usecs":      true,
				"controller_avg_write_io_latency_usecs":     true,
				// Calculated
				METRIC_TOTAL_WRITE_IO_SIZE: true,
			},
		},
	}
}
