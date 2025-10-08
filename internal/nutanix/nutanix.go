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
	//	"os"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	PRISM_API_PATH_VERSION_V1     = "v1/"
	PRISM_API_PATH_VERSION_V2     = "v2.0/"
	HTTP_TIMEOUT                  = 10 * time.Second
	MAX_PARALLEL_REQUESTS_DEFAULT = 10
)

type RequestParams struct {
	body, header string
	params       url.Values
}

type Nutanix struct {
	url                 string
	username            string
	password            string
	maxParallelRequests int
}

// ---- Cluster Health API ----

// ClusterHealthResponse represents Nutanix cluster health info
type ClusterHealthResponse struct {
	Status      string `json:"status"`
	ClusterName string `json:"cluster_name"`
}

// DetailedHealthResponse represents comprehensive health information
type DetailedHealthResponse struct {
	ClusterHealth ClusterHealthResponse `json:"cluster_health"`
	NodeHealth    []NodeHealthInfo      `json:"node_health"`
	StorageHealth StorageHealthInfo     `json:"storage_health"`
	NetworkHealth NetworkHealthInfo     `json:"network_health"`
}

// NodeHealthInfo represents health status of individual nodes
type NodeHealthInfo struct {
	UUID          string  `json:"uuid"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	CPUUsage      float64 `json:"cpu_usage"`
	MemoryUsage   float64 `json:"memory_usage"`
	DiskUsage     float64 `json:"disk_usage"`
	NetworkStatus string  `json:"network_status"`
}

// StorageHealthInfo represents storage subsystem health
type StorageHealthInfo struct {
	Status         string  `json:"status"`
	CapacityBytes  float64 `json:"capacity_bytes"`
	UsedBytes      float64 `json:"used_bytes"`
	AvailableBytes float64 `json:"available_bytes"`
	IOPS           float64 `json:"iops"`
	Latency        float64 `json:"latency"`
	Throughput     float64 `json:"throughput"`
}

// NetworkHealthInfo represents network subsystem health
type NetworkHealthInfo struct {
	Status         string  `json:"status"`
	TotalBandwidth float64 `json:"total_bandwidth"`
	UsedBandwidth  float64 `json:"used_bandwidth"`
	PacketLoss     float64 `json:"packet_loss"`
	Latency        float64 `json:"latency"`
}

// GetClusterHealth queries the Nutanix Prism API for overall cluster health
func (g *Nutanix) GetClusterHealth() (*ClusterHealthResponse, error) {
	resp, err := g.makeV2Request("GET", "cluster", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Name          string `json:"name"`
		OperationMode string `json:"operation_mode"`
		Status        string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Errorf("failed to parse cluster health response: %v", err)
		return nil, err
	}

	// Determine health status based on operation mode and status
	healthStatus := "OK"
	if result.OperationMode != "NORMAL" || result.Status != "ONLINE" {
		healthStatus = "WARNING"
	}
	if result.Status == "OFFLINE" {
		healthStatus = "CRITICAL"
	}

	return &ClusterHealthResponse{
		Status:      healthStatus,
		ClusterName: result.Name,
	}, nil
}

// GetDetailedHealth queries multiple Nutanix Prism API endpoints for comprehensive health information
func (g *Nutanix) GetDetailedHealth() (*DetailedHealthResponse, error) {
	detailedHealth := &DetailedHealthResponse{}

	// Get cluster health
	clusterHealth, err := g.GetClusterHealth()
	if err != nil {
		log.Errorf("failed to get cluster health: %v", err)
		return nil, err
	}
	detailedHealth.ClusterHealth = *clusterHealth

	// Get node health information
	nodeHealth, err := g.GetNodeHealth()
	if err != nil {
		log.Errorf("failed to get node health: %v", err)
	} else {
		detailedHealth.NodeHealth = nodeHealth
	}

	// Get storage health information
	storageHealth, err := g.GetStorageHealth()
	if err != nil {
		log.Errorf("failed to get storage health: %v", err)
	} else {
		detailedHealth.StorageHealth = *storageHealth
	}

	// Get network health information
	networkHealth, err := g.GetNetworkHealth()
	if err != nil {
		log.Errorf("failed to get network health: %v", err)
	} else {
		detailedHealth.NetworkHealth = *networkHealth
	}

	return detailedHealth, nil
}

// GetNodeHealth queries the Nutanix Prism API for node health information
func (g *Nutanix) GetNodeHealth() ([]NodeHealthInfo, error) {
	entities, err := g.fetchAllPages("hosts", nil)
	if err != nil {
		log.Errorf("failed to fetch hosts: %v", err)
		return nil, err
	}

	var nodeHealth []NodeHealthInfo
	for _, entityRaw := range entities {
		entity := entityRaw.(map[string]interface{})

		uuid, _ := entity["uuid"].(string)
		name, _ := entity["name"].(string)
		status, _ := entity["status"].(string)

		// Extract stats if available
		var cpuUsage, memoryUsage, diskUsage float64
		if stats, ok := entity["stats"].(map[string]interface{}); ok {
			if val, exists := stats["hypervisor_cpu_usage_ppm"]; exists {
				cpuUsage = g.valueToFloat64(val)
			}
			if val, exists := stats["hypervisor_memory_usage_ppm"]; exists {
				memoryUsage = g.valueToFloat64(val)
			}
			if val, exists := stats["hypervisor_storage_usage_ppm"]; exists {
				diskUsage = g.valueToFloat64(val)
			}
		}

		nodeHealth = append(nodeHealth, NodeHealthInfo{
			UUID:          uuid,
			Name:          name,
			Status:        status,
			CPUUsage:      cpuUsage,
			MemoryUsage:   memoryUsage,
			DiskUsage:     diskUsage,
			NetworkStatus: "OK", // Default value, could be enhanced with actual network checks
		})
	}

	return nodeHealth, nil
}

// GetStorageHealth queries the Nutanix Prism API for storage health information
func (g *Nutanix) GetStorageHealth() (*StorageHealthInfo, error) {
	resp, err := g.makeV2Request("GET", "cluster", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Errorf("failed to parse storage health response: %v", err)
		return nil, err
	}

	// Extract stats from the response
	var stats map[string]interface{}
	if statsRaw, ok := result["stats"]; ok {
		stats = statsRaw.(map[string]interface{})
	} else {
		log.Debugf("No stats found in cluster response")
		return &StorageHealthInfo{
			Status:         "UNKNOWN",
			CapacityBytes:  0,
			UsedBytes:      0,
			AvailableBytes: 0,
			IOPS:           0,
			Latency:        0,
			Throughput:     0,
		}, nil
	}

	// Get storage metrics - handle both string and numeric values
	capacityBytes := g.valueToFloat64(stats["storage.capacity_bytes"])
	usedBytes := g.valueToFloat64(stats["storage.usage_bytes"])
	iops := g.valueToFloat64(stats["controller_num_read_io"])
	latency := g.valueToFloat64(stats["controller_avg_read_io_latency_usecs"])
	throughput := g.valueToFloat64(stats["controller_total_read_io_size_kbytes"])

	log.Debugf("Storage health stats - capacity: %f, used: %f, iops: %f, latency: %f, throughput: %f",
		capacityBytes, usedBytes, iops, latency, throughput)

	availableBytes := capacityBytes - usedBytes
	status := "OK"
	if capacityBytes > 0 {
		utilization := usedBytes / capacityBytes
		if utilization > 0.9 {
			status = "WARNING"
		}
		if utilization > 0.95 {
			status = "CRITICAL"
		}
	}

	return &StorageHealthInfo{
		Status:         status,
		CapacityBytes:  capacityBytes,
		UsedBytes:      usedBytes,
		AvailableBytes: availableBytes,
		IOPS:           iops,
		Latency:        latency,
		Throughput:     throughput,
	}, nil
}

// GetNetworkHealth queries the Nutanix Prism API for network health information
func (g *Nutanix) GetNetworkHealth() (*NetworkHealthInfo, error) {
	resp, err := g.makeV2Request("GET", "cluster", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Errorf("failed to parse network health response: %v", err)
		return nil, err
	}

	// Extract stats from the response
	var stats map[string]interface{}
	if statsRaw, ok := result["stats"]; ok {
		stats = statsRaw.(map[string]interface{})
	}

	// Get network metrics
	totalBandwidth := g.valueToFloat64(stats["hypervisor_num_received_bytes"])
	usedBandwidth := g.valueToFloat64(stats["hypervisor_num_transmitted_bytes"])

	// Calculate bandwidth utilization
	bandwidthUtilization := 0.0
	if totalBandwidth > 0 {
		bandwidthUtilization = usedBandwidth / totalBandwidth
	}

	status := "OK"
	if bandwidthUtilization > 0.8 {
		status = "WARNING"
	}
	if bandwidthUtilization > 0.95 {
		status = "CRITICAL"
	}

	return &NetworkHealthInfo{
		Status:         status,
		TotalBandwidth: totalBandwidth,
		UsedBandwidth:  usedBandwidth,
		PacketLoss:     0.0, // Default value, could be enhanced with actual packet loss monitoring
		Latency:        0.0, // Default value, could be enhanced with actual latency monitoring
	}, nil
}

// valueToFloat64 converts given value to Float64
func (g *Nutanix) valueToFloat64(value interface{}) float64 {
	if value == nil {
		return 0.0
	}

	var v float64
	switch val := value.(type) {
	case float64:
		v = val
	case float32:
		v = float64(val)
	case int:
		v = float64(val)
	case int32:
		v = float64(val)
	case int64:
		v = float64(val)
	case string:
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			v = parsed
		} else {
			log.Debugf("Failed to parse string value '%s' as float64: %v", val, err)
			v = 0.0
		}
	default:
		log.Debugf("Unsupported type for valueToFloat64: %T, value: %v", value, value)
		v = 0.0
	}
	return v
}

func (g *Nutanix) makeV1Request(reqType string, action string, params url.Values) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V1, reqType, action, RequestParams{params: params})
}

func (g *Nutanix) makeV2Request(reqType string, action string, params url.Values) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V2, reqType, action, RequestParams{params: params})
}

func (g *Nutanix) makeRequestWithParams(versionPath, reqType, action string, p RequestParams) (*http.Response, error) {
	_url := strings.Trim(g.url, "/")
	_url += "/PrismGateway/services/rest/" + versionPath
	_url += strings.Trim(action, "/") + "/"

	log.Debugf("URL: %s", _url)

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	var netClient = http.Client{
		Transport: tr,
		Timeout:   HTTP_TIMEOUT,
	}

	body := p.body

	if p.params != nil && len(p.params) > 0 {
		_url += "?" + p.params.Encode()
	}

	req, err := http.NewRequest(reqType, _url, strings.NewReader(body))
	if err != nil {
		log.Errorf("failed to create request; error=%v\n", err)
		return nil, err
	}
	//req.Header.Set("Content-Type", "text/JSON")

	req.SetBasicAuth(g.username, g.password)

	resp, err := netClient.Do(req)
	if err != nil {
		log.Errorf("failed to execute request; error=%v\n", err)
		return nil, err
	}

	if resp.StatusCode >= 400 {
		log.Errorf("error status from server; status=%v code=%v\n", resp.Status, resp.StatusCode)
		return nil, fmt.Errorf("error status received")
	}

	return resp, nil
}

func NewNutanix(url, username, password string, maxParallelReq int) *Nutanix {
	nu := Nutanix{
		url:                 url,
		username:            username,
		password:            password,
		maxParallelRequests: maxParallelReq,
	}
	if nu.maxParallelRequests <= 0 {
		nu.maxParallelRequests = MAX_PARALLEL_REQUESTS_DEFAULT
	}
	log.Debugf("Max parallel request count is set to %d", nu.maxParallelRequests)
	return &nu
}
