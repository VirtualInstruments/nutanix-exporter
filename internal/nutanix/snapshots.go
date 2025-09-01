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
	entities, err := e.api.fetchAllPages("/snapshots", nil)
	if err != nil {
		e.result = nil
		log.Error("Snapshots discovery failed")
		return
	}

	e.result = map[string]interface{}{"entities": entities}

	g := e.metrics["count"].WithLabelValues()
	g.Set(float64(len(entities)))
	g.Collect(ch)

	log.Debugf("Results: %d", len(entities))
	for _, ent := range entities {
		vm_details := ent["vm_create_spec"].(map[string]interface{})

		snapshot_name := ent["snapshot_name"].(string)
		snapshot_uuid := ent["uuid"].(string)
		vm_uuid := ent["vm_uuid"].(string)
		vm_name := vm_details["name"].(string)

		for _, key := range e.fields {
			g := e.metrics[key].WithLabelValues(snapshot_uuid, snapshot_name, vm_uuid, vm_name)
			g.Set(e.valueToFloat64(ent[key]))
			g.Collect(ch)
		}
		log.Debugf("Snapshot data collected for name=%s, uuid=%s", snapshot_name, snapshot_uuid)
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
