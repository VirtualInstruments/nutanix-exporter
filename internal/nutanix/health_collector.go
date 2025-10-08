package nutanix

import (
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// HealthCollector collects comprehensive Nutanix cluster health metrics
type HealthCollector struct {
	client *Nutanix
	
	// Cluster health metrics
	clusterHealth *prometheus.Desc
	
	// Node health metrics
	nodeHealthStatus *prometheus.Desc
	nodeCPUUsage     *prometheus.Desc
	nodeMemoryUsage  *prometheus.Desc
	nodeDiskUsage    *prometheus.Desc
	
	// Storage health metrics
	storageHealthStatus *prometheus.Desc
	storageCapacity     *prometheus.Desc
	storageUsed         *prometheus.Desc
	storageAvailable    *prometheus.Desc
	storageIOPS         *prometheus.Desc
	storageLatency      *prometheus.Desc
	storageThroughput   *prometheus.Desc
	
	// Network health metrics
	networkHealthStatus *prometheus.Desc
	networkBandwidth    *prometheus.Desc
	networkUtilization  *prometheus.Desc
	networkPacketLoss   *prometheus.Desc
	networkLatency      *prometheus.Desc
}

// NewHealthCollector creates a new Prometheus collector for comprehensive Nutanix health metrics
func NewHealthCollector(client *Nutanix) *HealthCollector {
	return &HealthCollector{
		client: client,
		
		// Cluster health
		clusterHealth: prometheus.NewDesc(
			"nutanix_cluster_health_status",
			"Overall health status of the Nutanix cluster (0=OK,1=Warning,2=Critical,3=Unknown)",
			[]string{"cluster_name"}, nil,
		),
		
		// Node health
		nodeHealthStatus: prometheus.NewDesc(
			"nutanix_node_health_status",
			"Health status of individual nodes (0=OK,1=Warning,2=Critical,3=Unknown)",
			[]string{"node_uuid", "node_name"}, nil,
		),
		nodeCPUUsage: prometheus.NewDesc(
			"nutanix_node_cpu_usage_ppm",
			"CPU usage of individual nodes in parts per million",
			[]string{"node_uuid", "node_name"}, nil,
		),
		nodeMemoryUsage: prometheus.NewDesc(
			"nutanix_node_memory_usage_ppm",
			"Memory usage of individual nodes in parts per million",
			[]string{"node_uuid", "node_name"}, nil,
		),
		nodeDiskUsage: prometheus.NewDesc(
			"nutanix_node_disk_usage_ppm",
			"Disk usage of individual nodes in parts per million",
			[]string{"node_uuid", "node_name"}, nil,
		),
		
		// Storage health
		storageHealthStatus: prometheus.NewDesc(
			"nutanix_storage_health_status",
			"Health status of storage subsystem (0=OK,1=Warning,2=Critical,3=Unknown)",
			[]string{"cluster_name"}, nil,
		),
		storageCapacity: prometheus.NewDesc(
			"nutanix_storage_capacity_bytes",
			"Total storage capacity in bytes",
			[]string{"cluster_name"}, nil,
		),
		storageUsed: prometheus.NewDesc(
			"nutanix_storage_used_bytes",
			"Used storage capacity in bytes",
			[]string{"cluster_name"}, nil,
		),
		storageAvailable: prometheus.NewDesc(
			"nutanix_storage_available_bytes",
			"Available storage capacity in bytes",
			[]string{"cluster_name"}, nil,
		),
		storageIOPS: prometheus.NewDesc(
			"nutanix_storage_iops",
			"Storage IOPS",
			[]string{"cluster_name"}, nil,
		),
		storageLatency: prometheus.NewDesc(
			"nutanix_storage_latency_usecs",
			"Storage latency in microseconds",
			[]string{"cluster_name"}, nil,
		),
		storageThroughput: prometheus.NewDesc(
			"nutanix_storage_throughput_kbytes",
			"Storage throughput in kilobytes",
			[]string{"cluster_name"}, nil,
		),
		
		// Network health
		networkHealthStatus: prometheus.NewDesc(
			"nutanix_network_health_status",
			"Health status of network subsystem (0=OK,1=Warning,2=Critical,3=Unknown)",
			[]string{"cluster_name"}, nil,
		),
		networkBandwidth: prometheus.NewDesc(
			"nutanix_network_bandwidth_bytes",
			"Network bandwidth in bytes",
			[]string{"cluster_name", "type"}, nil,
		),
		networkUtilization: prometheus.NewDesc(
			"nutanix_network_utilization_ratio",
			"Network utilization ratio (0-1)",
			[]string{"cluster_name"}, nil,
		),
		networkPacketLoss: prometheus.NewDesc(
			"nutanix_network_packet_loss_ratio",
			"Network packet loss ratio (0-1)",
			[]string{"cluster_name"}, nil,
		),
		networkLatency: prometheus.NewDesc(
			"nutanix_network_latency_usecs",
			"Network latency in microseconds",
			[]string{"cluster_name"}, nil,
		),
	}
}

func (c *HealthCollector) Describe(ch chan<- *prometheus.Desc) {
	// Cluster health
	ch <- c.clusterHealth
	
	// Node health
	ch <- c.nodeHealthStatus
	ch <- c.nodeCPUUsage
	ch <- c.nodeMemoryUsage
	ch <- c.nodeDiskUsage
	
	// Storage health
	ch <- c.storageHealthStatus
	ch <- c.storageCapacity
	ch <- c.storageUsed
	ch <- c.storageAvailable
	ch <- c.storageIOPS
	ch <- c.storageLatency
	ch <- c.storageThroughput
	
	// Network health
	ch <- c.networkHealthStatus
	ch <- c.networkBandwidth
	ch <- c.networkUtilization
	ch <- c.networkPacketLoss
	ch <- c.networkLatency
}

func (c *HealthCollector) Collect(ch chan<- prometheus.Metric) {
	// Get comprehensive health data
	data, err := c.client.GetDetailedHealth()
	if err != nil {
		log.Errorf("failed to get detailed health: %v", err)
		return
	}

	// Collect cluster health metrics
	clusterHealthValue := c.statusToValue(data.ClusterHealth.Status)
	ch <- prometheus.MustNewConstMetric(
		c.clusterHealth,
		prometheus.GaugeValue,
		clusterHealthValue,
		data.ClusterHealth.ClusterName,
	)

	// Collect node health metrics
	for _, node := range data.NodeHealth {
		nodeHealthValue := c.statusToValue(node.Status)
		ch <- prometheus.MustNewConstMetric(
			c.nodeHealthStatus,
			prometheus.GaugeValue,
			nodeHealthValue,
			node.UUID, node.Name,
		)
		
		ch <- prometheus.MustNewConstMetric(
			c.nodeCPUUsage,
			prometheus.GaugeValue,
			node.CPUUsage,
			node.UUID, node.Name,
		)
		
		ch <- prometheus.MustNewConstMetric(
			c.nodeMemoryUsage,
			prometheus.GaugeValue,
			node.MemoryUsage,
			node.UUID, node.Name,
		)
		
		ch <- prometheus.MustNewConstMetric(
			c.nodeDiskUsage,
			prometheus.GaugeValue,
			node.DiskUsage,
			node.UUID, node.Name,
		)
	}

	// Collect storage health metrics
	storageHealthValue := c.statusToValue(data.StorageHealth.Status)
	ch <- prometheus.MustNewConstMetric(
		c.storageHealthStatus,
		prometheus.GaugeValue,
		storageHealthValue,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.storageCapacity,
		prometheus.GaugeValue,
		data.StorageHealth.CapacityBytes,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.storageUsed,
		prometheus.GaugeValue,
		data.StorageHealth.UsedBytes,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.storageAvailable,
		prometheus.GaugeValue,
		data.StorageHealth.AvailableBytes,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.storageIOPS,
		prometheus.GaugeValue,
		data.StorageHealth.IOPS,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.storageLatency,
		prometheus.GaugeValue,
		data.StorageHealth.Latency,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.storageThroughput,
		prometheus.GaugeValue,
		data.StorageHealth.Throughput,
		data.ClusterHealth.ClusterName,
	)

	// Collect network health metrics
	networkHealthValue := c.statusToValue(data.NetworkHealth.Status)
	ch <- prometheus.MustNewConstMetric(
		c.networkHealthStatus,
		prometheus.GaugeValue,
		networkHealthValue,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.networkBandwidth,
		prometheus.GaugeValue,
		data.NetworkHealth.TotalBandwidth,
		data.ClusterHealth.ClusterName, "total",
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.networkBandwidth,
		prometheus.GaugeValue,
		data.NetworkHealth.UsedBandwidth,
		data.ClusterHealth.ClusterName, "used",
	)
	
	// Calculate network utilization
	networkUtilization := 0.0
	if data.NetworkHealth.TotalBandwidth > 0 {
		networkUtilization = data.NetworkHealth.UsedBandwidth / data.NetworkHealth.TotalBandwidth
	}
	
	ch <- prometheus.MustNewConstMetric(
		c.networkUtilization,
		prometheus.GaugeValue,
		networkUtilization,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.networkPacketLoss,
		prometheus.GaugeValue,
		data.NetworkHealth.PacketLoss,
		data.ClusterHealth.ClusterName,
	)
	
	ch <- prometheus.MustNewConstMetric(
		c.networkLatency,
		prometheus.GaugeValue,
		data.NetworkHealth.Latency,
		data.ClusterHealth.ClusterName,
	)
}

// statusToValue converts status string to numeric value for Prometheus metrics
func (c *HealthCollector) statusToValue(status string) float64 {
	switch status {
	case "OK", "Healthy", "UP":
		return 0
	case "WARNING", "Degraded", "WARN":
		return 1
	case "CRITICAL", "Error", "DOWN", "CRIT":
		return 2
	default:
		return 3
	}
}
