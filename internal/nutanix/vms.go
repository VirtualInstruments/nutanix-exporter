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
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const (
	KEY_VM_PROPERTIES           = "properties"
	METRIC_MEM_FREE_BYTES       = "memory_free_bytes"
	METRIC_MEM_USAGE_BYTES      = "memory_usage_bytes"
	METRIC_MEM_SWAPPED_IN_RATE  = "memory_swapped_in_rate_bps"
	METRIC_MEM_SWAPPED_OUT_RATE = "memory_swapped_out_rate_bps"
)

// VmsExporter
type VmsExporter struct {
	*nutanixExporter
	networkExporters map[string]*VMNicsExporter
	collectvmnics    bool
}

// Describe - Implement prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *VmsExporter) Describe(ch chan<- *prometheus.Desc) {
	resp, err := e.api.makeV1Request("GET", "/vms/")
	if err != nil {
		e.result = nil
		log.Error("VM discovery failed")
		return
	}

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

	for _, entity := range entities {
		ent := entity.(map[string]interface{})
		var stats map[string]interface{} = nil
		if obj, ok := ent["stats"]; ok {
			stats = obj.(map[string]interface{})
		}

		if e.collectvmnics {
			var vmName string
			if obj, ok := ent["vmName"]; ok {
				vmName = obj.(string)
			}
			if obj, ok := ent["uuid"]; ok {
				uuid := obj.(string)
				e.networkExporters[uuid] = NewVMsNetworkCollector(&e.api, vmName, uuid)
			}
		}

		if stats != nil {
			e.addCalculatedStats(ent, stats)
			for key := range stats {
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}

				key = e.normalizeKey(key)

				e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: e.namespace,
					Name:      key, Help: "..."}, []string{"uuid", "host_uuid"})

				e.metrics[key].Describe(ch)
			}
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

	e.DescribeNicsParallel(ch)
}

func (e *VmsExporter) addCalculatedStats(ent map[string]interface{}, stats map[string]interface{}) {
	if stats == nil {
		return
	}
	// Add free memory stat
	mem_total := e.valueToFloat64(ent["memoryCapacityInBytes"])
	var mem_usage float64 = 0
	val, ok := stats["guest.memory_usage_bytes"]
	if ok {
		v := e.valueToFloat64(val)
		if v > 0 {
			mem_usage = v
		}
	}
	stats[METRIC_MEM_FREE_BYTES] = mem_total - mem_usage
	// add swapped in rate stat
	var mem_swapped_in_bytes, mem_swapped_out_bytes, controller_timespan_usecs float64 = 0, 0, 0
	val, ok = stats["guest.memory_swapped_in_bytes"]
	if ok {
		v := e.valueToFloat64(val)
		if v > 0 {
			mem_swapped_in_bytes = v
		}
	}
	val, ok = stats["guest.memory_swapped_out_bytes"]
	if ok {
		v := e.valueToFloat64(val)
		if v > 0 {
			mem_swapped_out_bytes = v
		}
	}
	val, ok = stats["controller_timespan_usecs"]
	if ok {
		v := e.valueToFloat64(val)
		if v > 0 {
			controller_timespan_usecs = v
		}
	}
	if controller_timespan_usecs > 0 {
		stats[METRIC_MEM_SWAPPED_IN_RATE] = (mem_swapped_in_bytes * 1000000) / controller_timespan_usecs
		stats[METRIC_MEM_SWAPPED_OUT_RATE] = (mem_swapped_out_bytes * 1000000) / controller_timespan_usecs
	} else {
		stats[METRIC_MEM_SWAPPED_IN_RATE] = 0
		stats[METRIC_MEM_SWAPPED_OUT_RATE] = 0
	}
}

// Collect - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *VmsExporter) Collect(ch chan<- prometheus.Metric) {
	if e.result == nil {
		return
	}
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
			var val string = ""
			// format properties
			switch property {
			case "memoryCapacityInMB", "memoryReservedCapacityInMB", "diskCapacityInMB":
				propname := strings.Replace(property, "MB", "Bytes", 1)
				obj := ent[propname]
				if obj != nil {
					floatval := e.valueToFloat64(obj)
					floatval = floatval / (1024 * 1024)
					val = strconv.FormatFloat(floatval, 'f', 0, 64)
				}
			case "cpuReservedInMHz":
				propname := strings.Replace(property, "MHz", "Hz", 1)
				obj := ent[propname]
				if obj != nil {
					floatval := e.valueToFloat64(obj)
					floatval = floatval / 1000000
					val = strconv.FormatFloat(floatval, 'f', 0, 64)
				}
			case "numVCpus":
				obj := ent[property]
				if obj != nil {
					floatval := e.valueToFloat64(obj)
					val = strconv.FormatFloat(floatval, 'f', 0, 64)
				}
			case "ipAddresses":
				obj := ent[property]
				if obj != nil {
					strarr := []string{}
					for _, addr := range obj.([]interface{}) {
						strarr = append(strarr, addr.(string))
					}
					val = strings.Join(strarr, ",")
				}
			case "controllerVm":
				if obj, ok := ent[property].(bool); ok {
					val = strconv.FormatBool(obj) // Convert bool to string
				}
			default:
				obj := ent[property]
				if obj != nil {
					val = ent[property].(string)
				}
			}
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
				if _, ok := e.filter_stats[key]; !ok {
					continue
				}
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
			normalized_key := e.normalizeKey(key)
			log.Debugf("Collect Key %s", key)

			g = e.metrics[normalized_key].WithLabelValues(ent["uuid"].(string), hostUUID)

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
	log.Debug("VMs data collected")

	for vmUUID, networkExporter := range e.networkExporters {
		log.Debugf("Collect nic metrics for vm UUID: %s", vmUUID)
		networkExporter.Collect(ch)
	}
}

// NewVmsCollector - Create the Collector for VMs
func NewVmsCollector(_api *Nutanix, collectvmnics bool) *VmsExporter {

	return &VmsExporter{
		networkExporters: make(map[string]*VMNicsExporter),
		collectvmnics:    collectvmnics,
		nutanixExporter: &nutanixExporter{
			api:        *_api,
			metrics:    make(map[string]*prometheus.GaugeVec),
			namespace:  "nutanix_vms",
			fields:     []string{"memoryCapacityInBytes", "numVCpus", "powerState", "cpuReservedInHz"},
			properties: []string{"uuid", "hostUuid", "vmName", "memoryCapacityInMB", "memoryReservedCapacityInMB", "numVCpus", "powerState", "cpuReservedInMHz", "diskCapacityInMB", "ipAddresses", "controllerVm"},
			filter_stats: map[string]bool{
				"hypervisor_cpu_usage_ppm":         true,
				"guest.memory_usage_bytes":         true,
				"hypervisor_num_received_bytes":    true,
				"hypervisor_num_transmitted_bytes": true,
				"hypervisor.cpu_ready_time_ppm":    true,
				// The swapped in and out bytes metrics are collected on timestamp different that collection interval. So not publishing
				//"guest.memory_swapped_in_bytes":    true,
				//"guest.memory_swapped_out_bytes":   true,
				// Calculated
				METRIC_MEM_FREE_BYTES:       true,
				METRIC_MEM_SWAPPED_IN_RATE:  true,
				METRIC_MEM_SWAPPED_OUT_RATE: true,
				"controllerVm":              true,
			},
		}}
}

func (e *VmsExporter) DescribeNicsParallel(ch chan<- *prometheus.Desc) {
	var wg sync.WaitGroup
	// Create a buffered channel to limit concurrent Describe calls
	semaphore := make(chan struct{}, e.api.maxParallelRequests)
	for vmUUID, networkExporter := range e.networkExporters {
		wg.Add(1)
		go func(vmUUID string, exporter *VMNicsExporter) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire a token
			defer func() { <-semaphore }() // Release the token
			log.Debugf("Describing vm nic metrics for vm UUID: %s", vmUUID)
			exporter.Describe(ch)
		}(vmUUID, networkExporter)
	}
	wg.Wait()
}
