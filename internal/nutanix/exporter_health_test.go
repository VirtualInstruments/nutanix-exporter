package nutanix

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestExporterHealthCollector(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	collector := NewExporterHealthCollector("test-section", "test-uuid")

	// Test Describe
	descCh := make(chan *prometheus.Desc, 20)
	collector.Describe(descCh)
	close(descCh)

	descs := make([]*prometheus.Desc, 0)
	for desc := range descCh {
		descs = append(descs, desc)
	}

	// Should have 13 descriptors (all health metrics)
	assert.Len(t, descs, 13)

	// Test Collect with initial values
	metricCh := make(chan prometheus.Metric, 20)
	collector.Collect(metricCh)
	close(metricCh)

	metrics := make([]prometheus.Metric, 0)
	for metric := range metricCh {
		metrics = append(metrics, metric)
	}

	// Should have 13 metrics
	assert.Len(t, metrics, 13)

	// Verify all metrics have correct descriptors
	for _, metric := range metrics {
		desc := metric.Desc()
		assert.NotNil(t, desc)
		assert.Contains(t, desc.String(), "nutanix_exporter_")
	}
}

func TestMarkCollectionStart(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"

	// First collection should start successfully
	started := MarkCollectionStart(section)
	assert.True(t, started)

	// Second collection should fail (already active)
	started = MarkCollectionStart(section)
	assert.False(t, started)

	// Verify error counter was incremented
	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, uint64(1), h.errCollectionStillRunning)
	h.mu.RUnlock()
}

func TestMarkCollectionEnd(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"

	// Start a collection
	MarkCollectionStart(section)

	// End with success
	duration := 100 * time.Millisecond
	MarkCollectionEnd(section, true, duration)

	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, 0, h.activeCollections)
	assert.Equal(t, uint64(100000), h.totalSuccessCollectionDurationUS) // 100ms in microseconds
	assert.Equal(t, uint64(1), h.successfulPCCallNoErrors)
	assert.Equal(t, uint64(0), h.failedCollections)
	h.mu.RUnlock()

	// Test failure case
	MarkCollectionStart(section)
	MarkCollectionEnd(section, false, duration)

	h.mu.RLock()
	assert.Equal(t, uint64(1), h.failedCollections)
	assert.Equal(t, uint64(100000), h.totalFailureCollectionDurationUS)
	h.mu.RUnlock()
}

func TestMarkCmdSuccess(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"
	duration := 50 * time.Millisecond

	MarkCmdSuccess(section, duration)

	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, uint64(1), h.successDeviceCmd)
	assert.Equal(t, uint64(50000), h.totalSuccessCmdExecDurationUS) // 50ms in microseconds
	h.mu.RUnlock()
}

func TestMarkCmdFailure(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"
	duration := 75 * time.Millisecond

	MarkCmdFailure(section, duration)

	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, uint64(1), h.failureDeviceCmd)
	assert.Equal(t, uint64(75000), h.totalFailureCmdExecDurationUS) // 75ms in microseconds
	h.mu.RUnlock()
}

func TestIncConnTimeout(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"

	IncConnTimeout(section)

	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, uint64(1), h.errConnTimeout)
	h.mu.RUnlock()
}

func TestIncDNSFailure(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"

	IncDNSFailure(section)

	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, uint64(1), h.errDNSFailure)
	h.mu.RUnlock()
}

func TestIncException(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"

	IncException(section)

	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, uint64(1), h.errException)
	h.mu.RUnlock()
}

func TestStartHealthTicker(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"

	// Create a health instance for the section
	getHealth(section)

	stopCh := make(chan struct{})
	intervalSeconds := 1 // Use 1 second for faster testing

	// Start the ticker
	StartHealthTicker(stopCh, intervalSeconds)

	// Wait for at least one tick
	time.Sleep(1200 * time.Millisecond)

	// Check that poll cycles were incremented
	h := getHealth(section)
	h.mu.RLock()
	assert.Greater(t, h.totalPollCycles, uint64(0))
	h.mu.RUnlock()

	// Stop the ticker
	close(stopCh)
	time.Sleep(100 * time.Millisecond) // Give it time to stop
}

func TestStartHealthTickerInvalidInterval(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	stopCh := make(chan struct{})

	// Test with invalid interval (should return early)
	StartHealthTicker(stopCh, -1)

	// Test with zero interval (should return early)
	StartHealthTicker(stopCh, 0)

	// Should not panic or create tickers
	close(stopCh)
}

func TestConcurrentAccess(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup

	// Test concurrent MarkCmdSuccess calls
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				MarkCmdSuccess(section, time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Verify final count
	h := getHealth(section)
	h.mu.RLock()
	expectedCount := uint64(numGoroutines * numOperations)
	assert.Equal(t, expectedCount, h.successDeviceCmd)
	h.mu.RUnlock()
}

func TestMultipleSections(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section1 := "section1"
	section2 := "section2"

	// Test that different sections have independent health states
	MarkCmdSuccess(section1, time.Millisecond)
	MarkCmdFailure(section2, time.Millisecond)

	h1 := getHealth(section1)
	h2 := getHealth(section2)

	h1.mu.RLock()
	h2.mu.RLock()

	assert.Equal(t, uint64(1), h1.successDeviceCmd)
	assert.Equal(t, uint64(0), h1.failureDeviceCmd)
	assert.Equal(t, uint64(0), h2.successDeviceCmd)
	assert.Equal(t, uint64(1), h2.failureDeviceCmd)

	h1.mu.RUnlock()
	h2.mu.RUnlock()
}

func TestGetHealth(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	section := "test-section"

	// First call should create new health instance
	h1 := getHealth(section)
	assert.NotNil(t, h1)

	// Second call should return same instance
	h2 := getHealth(section)
	assert.Equal(t, h1, h2)

	// Different section should return different instance
	h3 := getHealth("different-section")
	assert.NotNil(t, h3)

	// Test that instances are valid
	assert.NotNil(t, h1)
	assert.NotNil(t, h3)
}
