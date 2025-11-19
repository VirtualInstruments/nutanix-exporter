package nutanix

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ExporterHealth keeps exporter self-health counters and durations.
type ExporterHealth struct {
	mu sync.RWMutex

	// counters (monotonic)
	errConnTimeout            uint64
	errCollectionStillRunning uint64
	errException              uint64
	errDNSFailure             uint64
	successDeviceCmd          uint64
	failureDeviceCmd          uint64
	totalPollCycles           uint64
	successfulPCCallNoErrors  uint64
	failedCollections         uint64

	// durations (microseconds, totals)
	totalSuccessCmdExecDurationUS    uint64
	totalFailureCmdExecDurationUS    uint64
	totalSuccessCollectionDurationUS uint64
	totalFailureCollectionDurationUS uint64

	// internal state
	activeCollections int
	// Track command durations at collection start to calculate incremental duration per collection
	cmdExecDurationAtCollectionStart uint64 // Success command duration when collection started
	failureCmdExecDurationAtStart    uint64 // Failure command duration when collection started
}

// healthBySection keeps one health state per configuration/section
var (
	healthMu        sync.RWMutex
	healthBySection = map[string]*ExporterHealth{}
)

func getHealth(section string) *ExporterHealth {
	healthMu.Lock()
	defer healthMu.Unlock()
	h, ok := healthBySection[section]
	if !ok {
		h = &ExporterHealth{}
		healthBySection[section] = h
	}
	return h
}

// Exposed Prometheus descriptors - all include cluster_uuid, uuid, and section labels
var (
	descErrConnTimeout                   = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataConnectionTimeout_C", "Exporter: connection timeouts encountered while calling Prism API", []string{"cluster_uuid", "uuid", "section"}, nil)
	descErrCollectionStillRunning        = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataCollectionStillRunning_C", "Exporter: collection overlap occurrences", []string{"cluster_uuid", "uuid", "section"}, nil)
	descErrException                     = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataException_C", "Exporter: generic errors while calling Prism API", []string{"cluster_uuid", "uuid", "section"}, nil)
	descErrDNSFailure                    = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataDNSLookupFailure_C", "Exporter: DNS lookup failures", []string{"cluster_uuid", "uuid", "section"}, nil)
	descSuccessDeviceCmd                 = prometheus.NewDesc("nutanix_exporter_SuccessDeviceCommand_C", "Exporter: successful device/API commands", []string{"cluster_uuid", "uuid", "section"}, nil)
	descTotalSuccessCmdExecDurationUS    = prometheus.NewDesc("nutanix_exporter_TotalSuccessDeviceCmdExecDuration_US", "Exporter: total duration of successful API commands (microseconds)", []string{"cluster_uuid", "uuid", "section"}, nil)
	descTotalSuccessCollectionDurationUS = prometheus.NewDesc("nutanix_exporter_TotalSuccessDeviceCollectionDuration_US", "Exporter: total duration of successful collections (microseconds). Represents command execution time + processing overhead (response parsing, metric formatting, etc.). Always >= TotalSuccessDeviceCmdExecDuration_US.", []string{"cluster_uuid", "uuid", "section"}, nil)
	descFailureDeviceCmd                 = prometheus.NewDesc("nutanix_exporter_FailureDeviceCommand_C", "Exporter: failed device/API commands", []string{"cluster_uuid", "uuid", "section"}, nil)
	descTotalFailureCmdExecDurationUS    = prometheus.NewDesc("nutanix_exporter_TotalFailureDeviceCmdExecDuration_US", "Exporter: total duration of failed API commands (microseconds)", []string{"cluster_uuid", "uuid", "section"}, nil)
	descTotalFailureCollectionDurationUS = prometheus.NewDesc("nutanix_exporter_TotalFailureDeviceCollectionDuration_US", "Exporter: total duration of failed collections (microseconds). Represents command execution time + processing overhead. Always >= TotalFailureDeviceCmdExecDuration_US.", []string{"cluster_uuid", "uuid", "section"}, nil)
	descTotalPollCycles                  = prometheus.NewDesc("nutanix_exporter_TotalPollCycles_C", "Exporter: total poll cycles (cumulative counter, increments per completed collection)", []string{"cluster_uuid", "uuid", "section"}, nil)
	descSuccessfulPCCallNoErrors         = prometheus.NewDesc("nutanix_exporter_SuccessfulPCCallNoErrors_C", "Exporter: successful poll cycles with no errors", []string{"cluster_uuid", "uuid", "section"}, nil)
	descFailedCollections                = prometheus.NewDesc("nutanix_exporter_FailedCollections_C", "Exporter: failed collection attempts", []string{"cluster_uuid", "uuid", "section"}, nil)
)

// ExporterHealthCollector exposes ExporterHealth as Prometheus metrics
type ExporterHealthCollector struct{ section, uuid, clusterUUID string }

func NewExporterHealthCollector(section, uuid, clusterUUID string) *ExporterHealthCollector {
	return &ExporterHealthCollector{section: section, uuid: uuid, clusterUUID: clusterUUID}
}

func (c *ExporterHealthCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descErrConnTimeout
	ch <- descErrCollectionStillRunning
	ch <- descErrException
	ch <- descErrDNSFailure
	ch <- descSuccessDeviceCmd
	ch <- descTotalSuccessCmdExecDurationUS
	ch <- descTotalSuccessCollectionDurationUS
	ch <- descFailureDeviceCmd
	ch <- descTotalFailureCmdExecDurationUS
	ch <- descTotalFailureCollectionDurationUS
	ch <- descTotalPollCycles
	ch <- descSuccessfulPCCallNoErrors
	ch <- descFailedCollections
}

func (c *ExporterHealthCollector) Collect(ch chan<- prometheus.Metric) {
	h := getHealth(c.section)
	h.mu.RLock()
	defer h.mu.RUnlock()

	// All metrics now include cluster_uuid, uuid, and section labels (in that order)
	ch <- prometheus.MustNewConstMetric(descErrConnTimeout, prometheus.CounterValue, float64(h.errConnTimeout), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descErrCollectionStillRunning, prometheus.CounterValue, float64(h.errCollectionStillRunning), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descErrException, prometheus.CounterValue, float64(h.errException), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descErrDNSFailure, prometheus.CounterValue, float64(h.errDNSFailure), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descSuccessDeviceCmd, prometheus.CounterValue, float64(h.successDeviceCmd), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalSuccessCmdExecDurationUS, prometheus.CounterValue, float64(h.totalSuccessCmdExecDurationUS), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalSuccessCollectionDurationUS, prometheus.CounterValue, float64(h.totalSuccessCollectionDurationUS), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descFailureDeviceCmd, prometheus.CounterValue, float64(h.failureDeviceCmd), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalFailureCmdExecDurationUS, prometheus.CounterValue, float64(h.totalFailureCmdExecDurationUS), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalFailureCollectionDurationUS, prometheus.CounterValue, float64(h.totalFailureCollectionDurationUS), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalPollCycles, prometheus.CounterValue, float64(h.totalPollCycles), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descSuccessfulPCCallNoErrors, prometheus.CounterValue, float64(h.successfulPCCallNoErrors), c.clusterUUID, c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descFailedCollections, prometheus.CounterValue, float64(h.failedCollections), c.clusterUUID, c.uuid, c.section)
}

// StartHealthTicker is deprecated - no longer used.
// Poll cycles are now tracked based on actual collection completions in MarkCollectionEnd.
// Each scrape request from the Prometheus receiver represents one poll cycle.
// This function is kept for backward compatibility but does nothing.
func StartHealthTicker(stopCh <-chan struct{}, intervalSeconds int) {
	// Poll cycles are tracked automatically when MarkCollectionEnd is called
	// No ticker needed - the exporter is reactive and only runs on scrape requests
}

// Helpers used by main and Nutanix client to record events
func MarkCollectionStart(section string) bool {
	h := getHealth(section)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.activeCollections > 0 {
		h.errCollectionStillRunning++
		return false // Collection already active, don't start another
	}
	h.activeCollections++
	// Capture command durations at collection start to calculate incremental duration
	h.cmdExecDurationAtCollectionStart = h.totalSuccessCmdExecDurationUS
	h.failureCmdExecDurationAtStart = h.totalFailureCmdExecDurationUS
	return true // Collection started successfully
}

func MarkCollectionEnd(section string, success bool, duration time.Duration) {
	h := getHealth(section)
	h.mu.Lock()
	// Increment poll cycles - each completed collection is a poll cycle
	// This tracks actual scrape/collection cycles from Prometheus receiver
	h.totalPollCycles++
	if success {
		wallClockDurationUS := uint64(duration / time.Microsecond)
		// Calculate incremental command duration for this collection
		// This is the sum of all API call durations that occurred during this collection
		incrementalCmdDurationUS := h.totalSuccessCmdExecDurationUS - h.cmdExecDurationAtCollectionStart

		// Collection duration = command execution time + processing overhead
		// Processing overhead includes: response parsing, metric formatting, HTTP writing, etc.
		// - Sequential calls: wall-clock >= command duration, so processing = wall-clock - command duration
		// - Parallel calls: wall-clock < command duration (calls overlapped), so processing = wall-clock
		var processingOverheadUS uint64
		if wallClockDurationUS >= incrementalCmdDurationUS {
			// Sequential: processing overhead = wall-clock - command duration
			processingOverheadUS = wallClockDurationUS - incrementalCmdDurationUS
		} else {
			// Parallel: processing overhead = wall-clock (all API calls overlapped in time)
			processingOverheadUS = wallClockDurationUS
		}

		// Collection duration = command execution time + processing overhead
		collectionDurationUS := incrementalCmdDurationUS + processingOverheadUS
		h.totalSuccessCollectionDurationUS += collectionDurationUS

		h.successfulPCCallNoErrors++
	} else {
		wallClockDurationUS := uint64(duration / time.Microsecond)
		// Calculate incremental failure command duration for this collection
		incrementalFailureCmdDurationUS := h.totalFailureCmdExecDurationUS - h.failureCmdExecDurationAtStart

		// Same logic for failed collections
		var processingOverheadUS uint64
		if wallClockDurationUS >= incrementalFailureCmdDurationUS {
			// Sequential: processing overhead = wall-clock - command duration
			processingOverheadUS = wallClockDurationUS - incrementalFailureCmdDurationUS
		} else {
			// Parallel: processing overhead = wall-clock (all API calls overlapped in time)
			processingOverheadUS = wallClockDurationUS
		}

		// Collection duration = command execution time + processing overhead
		collectionDurationUS := incrementalFailureCmdDurationUS + processingOverheadUS
		h.totalFailureCollectionDurationUS += collectionDurationUS

		h.failedCollections++
	}
	if h.activeCollections > 0 {
		h.activeCollections--
	}
	h.mu.Unlock()
}

func MarkCmdSuccess(section string, d time.Duration) {
	h := getHealth(section)
	h.mu.Lock()
	h.successDeviceCmd++
	h.totalSuccessCmdExecDurationUS += uint64(d / time.Microsecond)
	h.mu.Unlock()
}

func MarkCmdFailure(section string, d time.Duration) {
	h := getHealth(section)
	h.mu.Lock()
	h.failureDeviceCmd++
	h.totalFailureCmdExecDurationUS += uint64(d / time.Microsecond)
	h.mu.Unlock()
}

func IncConnTimeout(section string) {
	h := getHealth(section)
	h.mu.Lock()
	h.errConnTimeout++
	h.mu.Unlock()
}
func IncDNSFailure(section string) {
	h := getHealth(section)
	h.mu.Lock()
	h.errDNSFailure++
	h.mu.Unlock()
}
func IncException(section string) {
	h := getHealth(section)
	h.mu.Lock()
	h.errException++
	h.mu.Unlock()
}
