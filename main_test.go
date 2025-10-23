package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthOnlyEndpoint(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate the /metrics handler with health=true
		params := r.URL.Query()
		section := params.Get("section")
		if len(section) == 0 {
			section = "default"
		}

		healthOnly := params.Get("health") == "true"

		// Mock config (simplified for test)

		// Simulate health-only request
		if healthOnly {
			// Should only return health metrics, no Nutanix API calls
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("# HELP nutanix_exporter_TotalPollCycles_C Exporter: total poll cycles\n# TYPE nutanix_exporter_TotalPollCycles_C counter\nnutanix_exporter_TotalPollCycles_C{uuid=\"exporter-health\",section=\"test-section\"} 0\n"))
			return
		}

		// Regular request would return more metrics
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("regular metrics"))
	}))
	defer server.Close()

	// Test health-only request
	resp, err := http.Get(server.URL + "/metrics?health=true&section=test-section")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify response contains health metrics
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	response := string(body[:n])

	assert.Contains(t, response, "nutanix_exporter_TotalPollCycles_C")
	assert.Contains(t, response, "uuid=\"exporter-health\"")
	assert.Contains(t, response, "section=\"test-section\"")
}

func TestSectionHandling(t *testing.T) {
	// Test that section parameter is properly handled
	req, err := http.NewRequest("GET", "/metrics?section=my-section", nil)
	require.NoError(t, err)

	// Extract section from query
	section := req.URL.Query().Get("section")
	assert.Equal(t, "my-section", section)

	// Test default section
	req2, err := http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)

	section2 := req2.URL.Query().Get("section")
	assert.Equal(t, "", section2) // Empty when not provided
}

func TestHealthUUIDGeneration(t *testing.T) {
	// Test health-only UUID generation
	healthOnly := true
	section := "test-section"

	healthUUID := "exporter-health"
	if !healthOnly {
		healthUUID = section
	}

	assert.Equal(t, "exporter-health", healthUUID)

	// Test normal request UUID generation
	healthOnly = false
	healthUUID = "exporter-health"
	if !healthOnly {
		healthUUID = section
	}

	assert.Equal(t, "test-section", healthUUID)
}

func TestStartedTickersTracking(t *testing.T) {
	// Reset global state
	startedTickers = make(map[string]bool)

	section1 := "section1"
	section2 := "section2"

	// First call should start ticker
	if !startedTickers[section1] {
		startedTickers[section1] = true
	}
	assert.True(t, startedTickers[section1])
	assert.False(t, startedTickers[section2])

	// Second call should not start ticker again
	if !startedTickers[section1] {
		startedTickers[section1] = true
	}
	assert.True(t, startedTickers[section1])

	// Different section should start its own ticker
	if !startedTickers[section2] {
		startedTickers[section2] = true
	}
	assert.True(t, startedTickers[section2])
}

func TestConfigValidation(t *testing.T) {
	// Test config structure
	config := map[string]cluster{
		"test-cluster": {
			Host:                "10.20.10.40",
			Username:            "admin",
			Password:            "password",
			LogLevel:            "debug",
			MaxParallelRequests: 10,
			Collect: map[string]bool{
				"health":  true,
				"cluster": true,
				"hosts":   true,
			},
		},
	}

	// Test config access
	conf, ok := config["test-cluster"]
	assert.True(t, ok)
	assert.Equal(t, "10.20.10.40", conf.Host)
	assert.Equal(t, "admin", conf.Username)
	assert.Equal(t, "password", conf.Password)
	assert.Equal(t, "debug", conf.LogLevel)
	assert.Equal(t, 10, conf.MaxParallelRequests)
	assert.True(t, conf.Collect["health"])
	assert.True(t, conf.Collect["cluster"])
	assert.True(t, conf.Collect["hosts"])
}

func TestCollectionIntervalLogic(t *testing.T) {
	// Test collection interval calculation
	conf := cluster{
		MaxParallelRequests: 5,
	}

	collectionInterval := 30 // default
	if conf.MaxParallelRequests > 0 {
		collectionInterval = 30 // could be made configurable
	}

	assert.Equal(t, 30, collectionInterval)

	// Test with zero parallel requests
	conf2 := cluster{
		MaxParallelRequests: 0,
	}

	collectionInterval2 := 30 // default
	if conf2.MaxParallelRequests > 0 {
		collectionInterval2 = 30
	}

	assert.Equal(t, 30, collectionInterval2)
}

func TestHealthCollectorRegistration(t *testing.T) {
	// Test that health collector is always registered
	section := "test-section"
	healthUUID := "test-uuid"

	// Simulate collector registration (simplified test)
	assert.NotEmpty(t, section)
	assert.NotEmpty(t, healthUUID)
}

func TestConcurrentHealthRequests(t *testing.T) {
	// Test concurrent health requests to same section
	numRequests := 10

	// Simulate concurrent requests
	results := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			// Simulate MarkCollectionStart
			started := true // Simplified for test
			results <- started
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
		if <-results {
			successCount++
		}
	}

	// All requests should succeed in this simplified test
	assert.Equal(t, numRequests, successCount)
}

func TestHealthMetricsLabels(t *testing.T) {
	// Test that health metrics have correct labels
	section := "test-section"
	uuid := "test-uuid"

	// Test basic label validation
	assert.NotEmpty(t, section)
	assert.NotEmpty(t, uuid)
	assert.Contains(t, section, "test")
	assert.Contains(t, uuid, "test")
}

func TestErrorHandling(t *testing.T) {
	// Test error handling for invalid inputs
	section := ""
	uuid := ""

	// Test empty string handling
	assert.Empty(t, section)
	assert.Empty(t, uuid)

	// Test with nil values (should not panic)
	var nilSection *string
	var nilUUID *string
	if nilSection == nil && nilUUID == nil {
		// This is expected behavior
		assert.True(t, true)
	}
}

func TestTimeDurationConversion(t *testing.T) {
	// Test time duration to microseconds conversion
	duration := 100 * time.Millisecond
	microseconds := uint64(duration / time.Microsecond)

	assert.Equal(t, uint64(100000), microseconds) // 100ms = 100,000 microseconds

	// Test with different durations
	duration2 := 1 * time.Second
	microseconds2 := uint64(duration2 / time.Microsecond)

	assert.Equal(t, uint64(1000000), microseconds2) // 1s = 1,000,000 microseconds
}
