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
	collecting bool
}

var exporterHealth = &ExporterHealth{}

// Exposed Prometheus descriptors
var (
	descErrConnTimeout                   = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataConnectionTimeout_C", "Exporter: connection timeouts encountered while calling Prism API", nil, nil)
	descErrCollectionStillRunning        = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataCollectionStillRunning_C", "Exporter: collection overlap occurrences", nil, nil)
	descErrException                     = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataException_C", "Exporter: generic errors while calling Prism API", nil, nil)
	descErrDNSFailure                    = prometheus.NewDesc("nutanix_exporter_ErrorPCNoDataDNSLookupFailure_C", "Exporter: DNS lookup failures", nil, nil)
	descSuccessDeviceCmd                 = prometheus.NewDesc("nutanix_exporter_SuccessDeviceCommand_C", "Exporter: successful device/API commands", nil, nil)
	descTotalSuccessCmdExecDurationUS    = prometheus.NewDesc("nutanix_exporter_TotalSuccessDeviceCmdExecDuration_US", "Exporter: total duration of successful API commands (microseconds)", nil, nil)
	descTotalSuccessCollectionDurationUS = prometheus.NewDesc("nutanix_exporter_TotalSuccessDeviceCollectionDuration_US", "Exporter: total duration of successful collections (microseconds)", nil, nil)
	descFailureDeviceCmd                 = prometheus.NewDesc("nutanix_exporter_FailureDeviceCommand_C", "Exporter: failed device/API commands", nil, nil)
	descTotalFailureCmdExecDurationUS    = prometheus.NewDesc("nutanix_exporter_TotalFailureDeviceCmdExecDuration_US", "Exporter: total duration of failed API commands (microseconds)", nil, nil)
	descTotalFailureCollectionDurationUS = prometheus.NewDesc("nutanix_exporter_TotalFailureDeviceCollectionDuration_US", "Exporter: total duration of failed collections (microseconds)", nil, nil)
	descTotalPollCycles                  = prometheus.NewDesc("nutanix_exporter_TotalPollCycles_C", "Exporter: total poll cycles (30s ticker)", nil, nil)
	descSuccessfulPCCallNoErrors         = prometheus.NewDesc("nutanix_exporter_SuccessfulPCCallNoErrors_C", "Exporter: successful poll cycles with no errors", nil, nil)
)

// ExporterHealthCollector exposes ExporterHealth as Prometheus metrics
type ExporterHealthCollector struct{}

func NewExporterHealthCollector() *ExporterHealthCollector { return &ExporterHealthCollector{} }

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
	exporterHealth.mu.RLock()
	defer exporterHealth.mu.RUnlock()

	ch <- prometheus.MustNewConstMetric(descErrConnTimeout, prometheus.CounterValue, float64(exporterHealth.errConnTimeout))
	ch <- prometheus.MustNewConstMetric(descErrCollectionStillRunning, prometheus.CounterValue, float64(exporterHealth.errCollectionStillRunning))
	ch <- prometheus.MustNewConstMetric(descErrException, prometheus.CounterValue, float64(exporterHealth.errException))
	ch <- prometheus.MustNewConstMetric(descErrDNSFailure, prometheus.CounterValue, float64(exporterHealth.errDNSFailure))
	ch <- prometheus.MustNewConstMetric(descSuccessDeviceCmd, prometheus.CounterValue, float64(exporterHealth.successDeviceCmd))
	ch <- prometheus.MustNewConstMetric(descTotalSuccessCmdExecDurationUS, prometheus.CounterValue, float64(exporterHealth.totalSuccessCmdExecDurationUS))
	ch <- prometheus.MustNewConstMetric(descTotalSuccessCollectionDurationUS, prometheus.CounterValue, float64(exporterHealth.totalSuccessCollectionDurationUS))
	ch <- prometheus.MustNewConstMetric(descFailureDeviceCmd, prometheus.CounterValue, float64(exporterHealth.failureDeviceCmd))
	ch <- prometheus.MustNewConstMetric(descTotalFailureCmdExecDurationUS, prometheus.CounterValue, float64(exporterHealth.totalFailureCmdExecDurationUS))
	ch <- prometheus.MustNewConstMetric(descTotalFailureCollectionDurationUS, prometheus.CounterValue, float64(exporterHealth.totalFailureCollectionDurationUS))
	ch <- prometheus.MustNewConstMetric(descTotalPollCycles, prometheus.CounterValue, float64(exporterHealth.totalPollCycles))
	ch <- prometheus.MustNewConstMetric(descSuccessfulPCCallNoErrors, prometheus.CounterValue, float64(exporterHealth.successfulPCCallNoErrors))
}

// HealthTicker increments poll cycles every 30s.
func StartHealthTicker(stopCh <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				exporterHealth.mu.Lock()
				exporterHealth.totalPollCycles++
				exporterHealth.mu.Unlock()
			case <-stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

// Helpers used by main and Nutanix client to record events
func MarkCollectionStart() bool {
	exporterHealth.mu.Lock()
	defer exporterHealth.mu.Unlock()
	if exporterHealth.collecting {
		exporterHealth.errCollectionStillRunning++
		return false
	}
	exporterHealth.collecting = true
	return true
}

func MarkCollectionEnd(success bool, duration time.Duration) {
	exporterHealth.mu.Lock()
	defer exporterHealth.mu.Unlock()
	if success {
		exporterHealth.totalSuccessCollectionDurationUS += uint64(duration / time.Microsecond)
		exporterHealth.successfulPCCallNoErrors++
	} else {
		exporterHealth.totalFailureCollectionDurationUS += uint64(duration / time.Microsecond)
	}
	exporterHealth.collecting = false
}

func MarkCmdSuccess(d time.Duration) {
	exporterHealth.mu.Lock()
	exporterHealth.successDeviceCmd++
	exporterHealth.totalSuccessCmdExecDurationUS += uint64(d / time.Microsecond)
	exporterHealth.mu.Unlock()
}

func MarkCmdFailure(d time.Duration) {
	exporterHealth.mu.Lock()
	exporterHealth.failureDeviceCmd++
	exporterHealth.totalFailureCmdExecDurationUS += uint64(d / time.Microsecond)
	exporterHealth.mu.Unlock()
}

func IncConnTimeout() {
	exporterHealth.mu.Lock()
	exporterHealth.errConnTimeout++
	exporterHealth.mu.Unlock()
}
func IncDNSFailure() {
	exporterHealth.mu.Lock()
	exporterHealth.errDNSFailure++
	exporterHealth.mu.Unlock()
}
func IncException() {
	exporterHealth.mu.Lock()
	exporterHealth.errException++
	exporterHealth.mu.Unlock()
}
