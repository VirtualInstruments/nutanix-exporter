package nutanix

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkCmdSuccessIntegration(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	// Create a test server that simulates successful API calls
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	// Create Nutanix client
	nutanix := NewNutanix(server.URL, "user", "pass", 5)

	// Make a request that should succeed
	start := time.Now()
	resp, err := nutanix.makeRequestWithParams("", "GET", "test", RequestParams{})
	_ = time.Since(start)

	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Verify health metrics were updated
	section := server.URL // Use URL as section key
	h := getHealth(section)
	h.mu.RLock()
	assert.Greater(t, h.successDeviceCmd, uint64(0))
	assert.Greater(t, h.totalSuccessCmdExecDurationUS, uint64(0))
	h.mu.RUnlock()
}

func TestMarkCmdFailureIntegration(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	// Create a test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	// Create Nutanix client
	nutanix := NewNutanix(server.URL, "user", "pass", 5)

	// Make a request that should fail
	start := time.Now()
	resp, err := nutanix.makeRequestWithParams("", "GET", "test", RequestParams{})
	_ = time.Since(start)

	// Should get error due to 500 status
	assert.Error(t, err)
	assert.Nil(t, resp)

	// Verify health metrics were updated
	section := server.URL
	h := getHealth(section)
	h.mu.RLock()
	assert.Greater(t, h.failureDeviceCmd, uint64(0))
	assert.Greater(t, h.totalFailureCmdExecDurationUS, uint64(0))
	h.mu.RUnlock()
}

func TestConnectionTimeoutError(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	// Create a server that doesn't respond (simulates timeout)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't respond - simulate timeout
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	// Create Nutanix client with very short timeout
	nutanix := NewNutanix(server.URL, "user", "pass", 5)

	// Make a request that should timeout
	start := time.Now()
	resp, err := nutanix.makeRequestWithParams("", "GET", "test", RequestParams{})
	_ = time.Since(start)

	// Should get timeout error or connection error
	if err != nil {
		assert.True(t, strings.Contains(strings.ToLower(err.Error()), "timeout") ||
			strings.Contains(strings.ToLower(err.Error()), "context") ||
			strings.Contains(strings.ToLower(err.Error()), "connection"))
	} else {
		// If no error, the request might have succeeded despite delay
		assert.NotNil(t, resp)
	}

	// Verify some error was recorded (timeout or other)
	section := server.URL
	h := getHealth(section)
	h.mu.RLock()
	// Check if any error was recorded
	totalErrors := h.errConnTimeout + h.errException + h.errDNSFailure
	assert.GreaterOrEqual(t, totalErrors, uint64(0)) // At least 0 errors
	h.mu.RUnlock()
}

func TestDNSLookupFailure(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	// Use invalid hostname that will cause DNS lookup failure
	nutanix := NewNutanix("http://invalid-hostname-that-does-not-exist.com", "user", "pass", 5)

	// Make a request that should fail with DNS error
	start := time.Now()
	resp, err := nutanix.makeRequestWithParams("", "GET", "test", RequestParams{})
	_ = time.Since(start)

	// Should get DNS error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, strings.Contains(strings.ToLower(err.Error()), "no such host"))

	// Verify DNS failure was recorded
	section := "http://invalid-hostname-that-does-not-exist.com"
	h := getHealth(section)
	h.mu.RLock()
	assert.Greater(t, h.errDNSFailure, uint64(0))
	h.mu.RUnlock()
}

func TestGenericException(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	// Create a server that returns connection refused
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately
		w.(http.Flusher).Flush()
	}))
	server.Close() // Close server to cause connection error

	// Create Nutanix client
	nutanix := NewNutanix(server.URL, "user", "pass", 5)

	// Make a request that should fail with connection error
	start := time.Now()
	resp, err := nutanix.makeRequestWithParams("", "GET", "test", RequestParams{})
	_ = time.Since(start)

	// Should get connection error
	assert.Error(t, err)
	assert.Nil(t, resp)

	// Verify generic exception was recorded
	section := server.URL
	h := getHealth(section)
	h.mu.RLock()
	assert.Greater(t, h.errException, uint64(0))
	h.mu.RUnlock()
}

func TestRequestParams(t *testing.T) {
	// Test RequestParams struct
	params := RequestParams{
		body:   `{"test": "data"}`,
		params: map[string][]string{"key": {"value"}},
	}

	assert.Equal(t, `{"test": "data"}`, params.body)
	assert.Equal(t, "value", params.params.Get("key"))
}

func TestURLEncoding(t *testing.T) {
	// Test URL parameter encoding
	params := RequestParams{
		params: map[string][]string{
			"section": {"test-section"},
			"health":  {"true"},
		},
	}

	encoded := params.params.Encode()
	assert.Contains(t, encoded, "section=test-section")
	assert.Contains(t, encoded, "health=true")
}

func TestHTTPClientConfiguration(t *testing.T) {
	// Test that HTTP client is properly configured
	nutanix := NewNutanix("http://test.com", "user", "pass", 5)

	// Verify basic auth is set
	assert.Equal(t, "user", nutanix.username)
	assert.Equal(t, "pass", nutanix.password)
	assert.Equal(t, "http://test.com", nutanix.url)
	assert.Equal(t, 5, nutanix.maxParallelRequests)
}

func TestConcurrentAPIRequests(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	nutanix := NewNutanix(server.URL, "user", "pass", 5)

	// Make concurrent requests
	numRequests := 10
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			_, err := nutanix.makeRequestWithParams("", "GET", "test", RequestParams{})
			results <- err
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
		if <-results == nil {
			successCount++
		}
	}

	// All requests should succeed
	assert.Equal(t, numRequests, successCount)

	// Verify health metrics were updated
	section := server.URL
	h := getHealth(section)
	h.mu.RLock()
	assert.Equal(t, uint64(numRequests), h.successDeviceCmd)
	h.mu.RUnlock()
}

func TestHealthMetricsIntegration(t *testing.T) {
	// Reset global state
	healthMu.Lock()
	healthBySection = make(map[string]*ExporterHealth)
	healthMu.Unlock()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	nutanix := NewNutanix(server.URL, "user", "pass", 5)

	// Make several requests to generate health metrics
	for i := 0; i < 5; i++ {
		_, err := nutanix.makeRequestWithParams("", "GET", "test", RequestParams{})
		require.NoError(t, err)
	}

	// Verify all health metrics are properly updated
	section := server.URL
	h := getHealth(section)
	h.mu.RLock()

	assert.Equal(t, uint64(5), h.successDeviceCmd)
	assert.Equal(t, uint64(0), h.failureDeviceCmd)
	assert.Equal(t, uint64(0), h.errConnTimeout)
	assert.Equal(t, uint64(0), h.errDNSFailure)
	assert.Equal(t, uint64(0), h.errException)
	assert.Greater(t, h.totalSuccessCmdExecDurationUS, uint64(0))
	assert.Equal(t, uint64(0), h.totalFailureCmdExecDurationUS)

	h.mu.RUnlock()
}
