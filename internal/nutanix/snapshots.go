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

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// SnapshotsExporter
type SnapshotsExporter struct {
	*nutanixExporter
}

// Describe - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *SnapshotsExporter) Describe(ch chan<- *prometheus.Desc) {

	e.metrics["count"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: e.namespace,
		Name:      "total",
		Help:      "Count Snapshots on the cluster"}, []string{})
	e.metrics["count"].Describe(ch)

	for _, key := range e.fields {
		e.metrics[key] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: e.namespace,
			Name:      key, Help: "..."}, []string{"snapshot_uuid", "snapshot_name", "vm_uuid", "vm_name"})

		e.metrics[key].Describe(ch)
	}
}

// Collect - Implemente prometheus.Collector interface
// See https://github.com/prometheus/client_golang/blob/master/prometheus/collector.go
func (e *SnapshotsExporter) Collect(ch chan<- prometheus.Metric) {
	var snapshots map[string]interface{}

	resp, err := e.api.makeV2Request("GET", "/snapshots/")
	if err != nil {
		e.result = nil
		log.Error("Snapshots discovery failed")
		return
	}

	data := json.NewDecoder(resp.Body)
	data.Decode(&snapshots)

	metadata, _ := snapshots["metadata"].(map[string]interface{})
	g := e.metrics["count"].WithLabelValues()
	g.Set(e.valueToFloat64(metadata["total_entities"]))
	g.Collect(ch)

	entities, _ := snapshots["entities"].([]interface{})
	log.Debugf("Results: %s", len(entities))
	for _, entity := range entities {
		ent := entity.(map[string]interface{})
		vm_details := ent["vm_create_spec"].(map[string]interface{})

		snapshot_name := ent["snapshot_name"].(string)
		snapshot_uuid := ent["uuid"].(string)
		vm_uuid := ent["vm_uuid"].(string)
		vm_name := vm_details["name"].(string)

		for _, key := range e.fields {
			log.Debugf("%s > %s", key, ent[key])
			g := e.metrics[key].WithLabelValues(snapshot_uuid, snapshot_name, vm_uuid, vm_name)
			g.Set(e.valueToFloat64(ent[key]))
			g.Collect(ch)
		}
	}

}

// NewHostsCollector
func NewSnapshotsCollector(_api *Nutanix) *SnapshotsExporter {

	return &SnapshotsExporter{
		&nutanixExporter{
			api:       *_api,
			metrics:   make(map[string]*prometheus.GaugeVec),
			namespace: "nutanix_snapshots",
			fields:    []string{"created_time"},
		}}
}
