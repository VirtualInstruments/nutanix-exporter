package nutanix

import (
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// HealthCollector collects Nutanix cluster health metrics
type HealthCollector struct {
	client        *Nutanix
	clusterHealth *prometheus.Desc
}

// NewHealthCollector creates a new Prometheus collector for Nutanix cluster health
func NewHealthCollector(client *Nutanix) *HealthCollector {
	return &HealthCollector{
		client: client,
		clusterHealth: prometheus.NewDesc(
			"nutanix_cluster_health_status",
			"Overall health status of the Nutanix cluster (0=OK,1=Warning,2=Critical,3=Unknown)",
			[]string{"cluster_name"}, nil,
		),
	}
}

func (c *HealthCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.clusterHealth
}

func (c *HealthCollector) Collect(ch chan<- prometheus.Metric) {
	data, err := c.client.GetClusterHealth()
	if err != nil {
		log.Errorf("failed to get cluster health: %v", err)
		return
	}

	var value float64
	switch data.Status {
	case "OK", "Healthy":
		value = 0
	case "WARNING", "Degraded":
		value = 1
	case "CRITICAL", "Error":
		value = 2
	default:
		value = 3
	}

	ch <- prometheus.MustNewConstMetric(
		c.clusterHealth,
		prometheus.GaugeValue,
		value,
		data.ClusterName,
	)
}
