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

	// durations (microseconds, totals)
	totalSuccessCmdExecDurationUS    uint64
	totalFailureCmdExecDurationUS    uint64
	totalSuccessCollectionDurationUS uint64
	totalFailureCollectionDurationUS uint64

	// internal state
	activeCollections int
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

// Exposed Prometheus descriptors with labels
var (
	descErrConnTimeout                   = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataConnectionTimeout_C", "Exporter: connection timeouts encountered while calling Prism API", []string{"uuid", "section"}, nil)
	descErrCollectionStillRunning        = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataCollectionStillRunning_C", "Exporter: collection overlap occurrences", []string{"uuid", "section"}, nil)
	descErrException                     = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataException_C", "Exporter: generic errors while calling Prism API", []string{"uuid", "section"}, nil)
	descErrDNSFailure                    = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataDNSLookupFailure_C", "Exporter: DNS lookup failures", []string{"uuid", "section"}, nil)
	descSuccessDeviceCmd                 = prometheus.NewDesc("nutanix_exporter_SuccessDeviceCommand_C", "Exporter: successful device/API commands", []string{"uuid", "section"}, nil)
	descTotalSuccessCmdExecDurationUS    = prometheus.NewDesc("nutanix_exporter_TotalSuccessDeviceCmdExecDuration_US", "Exporter: total duration of successful API commands (microseconds)", []string{"uuid", "section"}, nil)
	descTotalSuccessCollectionDurationUS = prometheus.NewDesc("nutanix_exporter_TotalSuccessDeviceCollectionDuration_US", "Exporter: total duration of successful collections (microseconds)", []string{"uuid", "section"}, nil)
	descFailureDeviceCmd                 = prometheus.NewDesc("nutanix_exporter_FailureDeviceCommand_C", "Exporter: failed device/API commands", []string{"uuid", "section"}, nil)
	descTotalFailureCmdExecDurationUS    = prometheus.NewDesc("nutanix_exporter_TotalFailureDeviceCmdExecDuration_US", "Exporter: total duration of failed API commands (microseconds)", []string{"uuid", "section"}, nil)
	descTotalFailureCollectionDurationUS = prometheus.NewDesc("nutanix_exporter_TotalFailureDeviceCollectionDuration_US", "Exporter: total duration of failed collections (microseconds)", []string{"uuid", "section"}, nil)
	descTotalPollCycles                  = prometheus.NewDesc("nutanix_exporter_TotalPollCycles_C", "Exporter: total poll cycles (30s ticker)", []string{"uuid", "section"}, nil)
	descSuccessfulPCCallNoErrors         = prometheus.NewDesc("nutanix_exporter_SuccessfulPCCallNoErrors_C", "Exporter: successful poll cycles with no errors", []string{"uuid", "section"}, nil)
)

// ExporterHealthCollector exposes ExporterHealth as Prometheus metrics
type ExporterHealthCollector struct{ section, uuid string }

func NewExporterHealthCollector(section, uuid string) *ExporterHealthCollector {
	return &ExporterHealthCollector{section: section, uuid: uuid}
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
}

func (c *ExporterHealthCollector) Collect(ch chan<- prometheus.Metric) {
	h := getHealth(c.section)
	h.mu.RLock()
	defer h.mu.RUnlock()

	ch <- prometheus.MustNewConstMetric(descErrConnTimeout, prometheus.CounterValue, float64(h.errConnTimeout), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descErrCollectionStillRunning, prometheus.CounterValue, float64(h.errCollectionStillRunning), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descErrException, prometheus.CounterValue, float64(h.errException), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descErrDNSFailure, prometheus.CounterValue, float64(h.errDNSFailure), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descSuccessDeviceCmd, prometheus.CounterValue, float64(h.successDeviceCmd), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalSuccessCmdExecDurationUS, prometheus.CounterValue, float64(h.totalSuccessCmdExecDurationUS), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalSuccessCollectionDurationUS, prometheus.CounterValue, float64(h.totalSuccessCollectionDurationUS), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descFailureDeviceCmd, prometheus.CounterValue, float64(h.failureDeviceCmd), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalFailureCmdExecDurationUS, prometheus.CounterValue, float64(h.totalFailureCmdExecDurationUS), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalFailureCollectionDurationUS, prometheus.CounterValue, float64(h.totalFailureCollectionDurationUS), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descTotalPollCycles, prometheus.CounterValue, float64(h.totalPollCycles), c.uuid, c.section)
	ch <- prometheus.MustNewConstMetric(descSuccessfulPCCallNoErrors, prometheus.CounterValue, float64(h.successfulPCCallNoErrors), c.uuid, c.section)
}

// HealthTicker increments poll cycles every 30s.
func StartHealthTicker(stopCh <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				healthMu.RLock()
				for _, h := range healthBySection {
					h.mu.Lock()
					h.totalPollCycles++
					h.mu.Unlock()
				}
				healthMu.RUnlock()
			case <-stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

// Helpers used by main and Nutanix client to record events
func MarkCollectionStart(section string) {
	h := getHealth(section)
	h.mu.Lock()
	if h.activeCollections > 0 {
		h.errCollectionStillRunning++
	}
	h.activeCollections++
	h.mu.Unlock()
}

func MarkCollectionEnd(section string, success bool, duration time.Duration) {
	h := getHealth(section)
	h.mu.Lock()
	if success {
		h.totalSuccessCollectionDurationUS += uint64(duration / time.Microsecond)
		h.successfulPCCallNoErrors++
	} else {
		h.totalFailureCollectionDurationUS += uint64(duration / time.Microsecond)
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
