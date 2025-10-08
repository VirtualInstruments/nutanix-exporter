
# Nutanix Exporter

A comprehensive Prometheus exporter for Nutanix clusters that collects cluster, host, VM, storage, and health metrics.

## Features

- **Cluster Metrics**: Overall cluster health, capacity, and performance metrics
- **Host Metrics**: Individual node health, CPU, memory, and disk usage
- **VM Metrics**: Virtual machine status, resource usage, and performance
- **Storage Metrics**: Storage containers, capacity, and performance metrics
- **Health Metrics**: Comprehensive health monitoring including:
  - Cluster health status
  - Node health status and resource utilization
  - Storage health and performance metrics
  - Network health and bandwidth utilization
- **Snapshot Metrics**: VM snapshot information
- **Virtual Disk Metrics**: Virtual disk status and performance

## Running the exporter

    nutanix_exporter -nutanix.url https://nutanix_cluster:9440 -nutanix.username <user> -nutanix.password <password>

    localhost:9405/metrics

## Running exporter with different sections

    nutanix_exporter -nutanix.conf ./config.yml

During the Query pass GET-Parameter Section

    localhost:9405/metrics?section=cluster01

## Configuration

### Basic Configuration
```
default:
  nutanix_host: https://nutanix.cluster.local:9440
  nutanix_user: prometheus
  nutanix_password: p@ssw0rd
  log_level: info
  max_parallel_requests: 10
  collect:
    cluster: true
    hosts: true
    vms: true
    storage_containers: true
    snapshots: true
    virtual_disks: true
    health: true  # Enable comprehensive health metrics

cluster02:
  nutanix_host: https://nutanix02.cluster.local:9440
  nutanix_user: prometheus
  nutanix_password: qwertz
  log_level: info
  max_parallel_requests: 10
  collect:
    cluster: true
    hosts: true
    vms: true
    storage_containers: true
    snapshots: true
    virtual_disks: true
    health: true  # Enable comprehensive health metrics
```

### Configuration Options

- `nutanix_host`: Nutanix Prism API endpoint URL
- `nutanix_user`: Username for API authentication
- `nutanix_password`: Password for API authentication
- `log_level`: Logging level (debug, info, trace)
- `max_parallel_requests`: Maximum number of parallel API requests
- `collect`: Enable/disable specific metric collectors:
  - `cluster`: Cluster-level metrics
  - `hosts`: Host/node metrics
  - `vms`: Virtual machine metrics
  - `storage_containers`: Storage container metrics
  - `snapshots`: Snapshot metrics
  - `virtual_disks`: Virtual disk metrics
  - `health`: Comprehensive health metrics

## Health Metrics

The health metrics collector provides comprehensive monitoring of your Nutanix cluster:

### Cluster Health Metrics
- `nutanix_cluster_health_status`: Overall cluster health status (0=OK, 1=Warning, 2=Critical, 3=Unknown)

### Node Health Metrics
- `nutanix_node_health_status`: Individual node health status
- `nutanix_node_cpu_usage_ppm`: CPU usage per node
- `nutanix_node_memory_usage_ppm`: Memory usage per node
- `nutanix_node_disk_usage_ppm`: Disk usage per node

### Storage Health Metrics
- `nutanix_storage_health_status`: Storage subsystem health status
- `nutanix_storage_capacity_bytes`: Total storage capacity
- `nutanix_storage_used_bytes`: Used storage capacity
- `nutanix_storage_available_bytes`: Available storage capacity
- `nutanix_storage_iops`: Storage IOPS
- `nutanix_storage_latency_usecs`: Storage latency
- `nutanix_storage_throughput_kbytes`: Storage throughput

### Network Health Metrics
- `nutanix_network_health_status`: Network subsystem health status
- `nutanix_network_bandwidth_bytes`: Network bandwidth (total/used)
- `nutanix_network_utilization_ratio`: Network utilization ratio
- `nutanix_network_packet_loss_ratio`: Network packet loss ratio
- `nutanix_network_latency_usecs`: Network latency

## Prometheus Configuration

### Nutanix Exporter Configuration
```
nutanix.cluster.local:
  nutanix_host: https://nutanix.cluster.local:9440
  nutanix_user: prometheus
  nutanix_password: p@ssw0rd
  log_level: info
  max_parallel_requests: 10
  collect:
    cluster: true
    hosts: true
    vms: true
    storage_containers: true
    snapshots: true
    virtual_disks: true
    health: true

nutanix02.cluster.local:
  nutanix_host: https://nutanix02.cluster.local:9440
  nutanix_user: prometheus
  nutanix_password: qwertz
  log_level: info
  max_parallel_requests: 10
  collect:
    cluster: true
    hosts: true
    vms: true
    storage_containers: true
    snapshots: true
    virtual_disks: true
    health: true
```

### Prometheus Scrape Configuration
```
scrape_configs:
  - job_name: nutanix_export
    metrics_path: /metrics
    static_configs:
    - targets:
      - nutanix.cluster.local
      - nutanix02.cluster.local
    relabel_configs:
    - source_labels: [__address__]
      target_label: __param_section
    - source_labels: [__address__]
      target_label: __param_target
    - source_labels: [__param_target]
      target_label: instance
    - target_label: __address__
      replacement: nutanix_exporter:9405
```

## Example Alerting Rules

Here are some example Prometheus alerting rules for the health metrics:

```yaml
groups:
- name: nutanix_health
  rules:
  - alert: NutanixClusterHealthCritical
    expr: nutanix_cluster_health_status == 2
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Nutanix cluster health is critical"
      description: "Cluster {{ $labels.cluster_name }} health status is critical"

  - alert: NutanixNodeHealthWarning
    expr: nutanix_node_health_status == 1
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Nutanix node health is degraded"
      description: "Node {{ $labels.node_name }} ({{ $labels.node_uuid }}) health status is degraded"

  - alert: NutanixStorageCapacityHigh
    expr: (nutanix_storage_used_bytes / nutanix_storage_capacity_bytes) > 0.9
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Nutanix storage capacity is high"
      description: "Storage utilization for cluster {{ $labels.cluster_name }} is {{ $value | humanizePercentage }}"

  - alert: NutanixNodeCPUHigh
    expr: nutanix_node_cpu_usage_ppm > 800000
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Nutanix node CPU usage is high"
      description: "Node {{ $labels.node_name }} CPU usage is {{ $value | humanizePercentage }}"
```

