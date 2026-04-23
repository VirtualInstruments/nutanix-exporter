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
	"strings"
	"sync"
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
	body   string
	params url.Values
}

type Nutanix struct {
	url                 string
	username            string
	password            string
	maxParallelRequests int
	httpClient          *http.Client
	httpClientOnce      sync.Once
}

func (g *Nutanix) makeV1Request(reqType string, action string, params url.Values) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V1, reqType, action, RequestParams{params: params})
}

func (g *Nutanix) makeV2Request(reqType string, action string, params url.Values) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V2, reqType, action, RequestParams{params: params})
}

// getHTTPClient returns a shared HTTP client with connection pooling
func (g *Nutanix) getHTTPClient() *http.Client {
	g.httpClientOnce.Do(func() {
		tr := &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:        100,              // Maximum idle connections across all hosts
			MaxIdleConnsPerHost: 10,               // Maximum idle connections per host
			IdleConnTimeout:     90 * time.Second, // Close idle connections after 90s
			DisableKeepAlives:   false,            // Enable HTTP keep-alive
		}
		g.httpClient = &http.Client{
			Transport: tr,
			Timeout:   HTTP_TIMEOUT,
		}
	})
	return g.httpClient
}

func (g *Nutanix) makeRequestWithParams(versionPath, reqType, action string, p RequestParams) (*http.Response, error) {
	_url := strings.Trim(g.url, "/")
	_url += "/PrismGateway/services/rest/" + versionPath
	_url += strings.Trim(action, "/") + "/"

	log.Debugf("URL: %s", _url)

	// Use shared HTTP client with connection pooling
	netClient := g.getHTTPClient()

	body := p.body

	if len(p.params) > 0 {
		_url += "?" + p.params.Encode()
	}

	req, err := http.NewRequest(reqType, _url, strings.NewReader(body))
	if err != nil {
		log.Errorf("failed to create request; error=%v\n", err)
		return nil, err
	}
	//req.Header.Set("Content-Type", "text/JSON")

	req.SetBasicAuth(g.username, g.password)

	start := time.Now()
	resp, err := netClient.Do(req)
	if err != nil {
		log.Errorf("failed to execute request; error=%v\n", err)
		// heuristics for health and detailed error messages
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			IncConnTimeout(g.url)
			SetLastError(g.url, fmt.Sprintf("Connection timeout - device '%s' not responding. Check network connectivity and ensure the Prism Central is accessible.", g.url))
		} else if strings.Contains(strings.ToLower(err.Error()), "no such host") {
			IncDNSFailure(g.url)
			SetLastError(g.url, fmt.Sprintf("DNS lookup failed - unable to resolve hostname for '%s'. Verify the endpoint URL is correct.", g.url))
		} else if strings.Contains(strings.ToLower(err.Error()), "connection refused") {
			IncException(g.url)
			SetLastError(g.url, fmt.Sprintf("Connection refused by '%s'. Verify the Prism Central service is running and the port is correct.", g.url))
		} else if strings.Contains(strings.ToLower(err.Error()), "certificate") || strings.Contains(strings.ToLower(err.Error()), "tls") {
			IncException(g.url)
			SetLastError(g.url, fmt.Sprintf("TLS/Certificate error connecting to '%s'. Check certificate configuration.", g.url))
		} else {
			IncException(g.url)
			SetLastError(g.url, fmt.Sprintf("Failed to connect to '%s': %s", g.url, err.Error()))
		}
		MarkCmdFailure(g.url, time.Since(start))
		return nil, err
	}

	if resp.StatusCode >= 400 {
		log.Errorf("error status from server; status=%v code=%v\n", resp.Status, resp.StatusCode)
		// Set detailed error message based on HTTP status code
		switch resp.StatusCode {
		case 401:
			SetLastError(g.url, fmt.Sprintf("Authentication failed (HTTP 401) - invalid username or password for endpoint '%s'.", g.url))
		case 403:
			SetLastError(g.url, fmt.Sprintf("Access forbidden (HTTP 403) - insufficient permissions for endpoint '%s'. Check user role and permissions.", g.url))
		case 404:
			SetLastError(g.url, fmt.Sprintf("Resource not found (HTTP 404) - API endpoint not available at '%s'. Verify API version compatibility.", g.url))
		case 429:
			SetLastError(g.url, fmt.Sprintf("Rate limited (HTTP 429) by endpoint '%s'. Too many requests - will retry.", g.url))
		case 500, 502, 503, 504:
			SetLastError(g.url, fmt.Sprintf("Server error (HTTP %d) from '%s'. The Prism Central may be overloaded or experiencing issues.", resp.StatusCode, g.url))
		default:
			SetLastError(g.url, fmt.Sprintf("HTTP error %d from '%s': %s", resp.StatusCode, g.url, resp.Status))
		}
		MarkCmdFailure(g.url, time.Since(start))
		return nil, fmt.Errorf("error status received")
	}

	// Clear any previous error on success
	ClearLastError(g.url)
	MarkCmdSuccess(g.url, time.Since(start))
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

// GetClusterUUID retrieves the cluster UUID from the Nutanix API
func (g *Nutanix) GetClusterUUID() (string, error) {
	resp, err := g.makeV2Request("GET", "/cluster/", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster info: %w", err)
	}
	defer resp.Body.Close()

	var clusterInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&clusterInfo); err != nil {
		return "", fmt.Errorf("failed to decode cluster info: %w", err)
	}

	uuid, ok := clusterInfo["uuid"].(string)
	if !ok {
		return "", fmt.Errorf("cluster UUID not found in response")
	}

	return uuid, nil
}
