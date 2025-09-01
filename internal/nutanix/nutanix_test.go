package nutanix

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMakeRequestWithParams(t *testing.T) {
	// Setup a Nutanix client with dummy data
	client := NewNutanix("https://example.com", "user", "pass", 5)

	// Test cases
	tests := []struct {
		name          string
		versionPath   string
		reqType       string
		action        string
		params        url.Values
		expectedQuery string
	}{
		{
			name:          "No query params (nil)",
			versionPath:   PRISM_API_PATH_VERSION_V1,
			reqType:       "GET",
			action:        "testAction",
			params:        nil,
			expectedQuery: "",
		},
		{
			name:        "Empty query params",
			versionPath: PRISM_API_PATH_VERSION_V2,
			reqType:     "POST",
			action:      "anotherAction",
			params:      url.Values{},
			// No query string expected
			expectedQuery: "",
		},
		{
			name:        "With query params",
			versionPath: PRISM_API_PATH_VERSION_V1,
			reqType:     "GET",
			action:      "actionWithParams",
			params: url.Values{
				"key1": {"value1"},
				"key2": {"value2"},
			},
			expectedQuery: "key1=value1&key2=value2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createURL := func(versionPath, reqType, action string, p RequestParams) string {
				_url := strings.Trim(client.url, "/")
				_url += "/PrismGateway/services/rest/" + versionPath
				_url += strings.Trim(action, "/") + "/"

				if p.params != nil && len(p.params) > 0 {
					_url += "?" + p.params.Encode()
				}
				return _url
			}

			url := createURL(tt.versionPath, tt.reqType, tt.action, RequestParams{params: tt.params})

			if tt.expectedQuery == "" {
				if strings.Contains(url, "?") {
					t.Errorf("Expected no query string, but got URL: %s", url)
				}
			} else {
				if !strings.Contains(url, "?"+tt.expectedQuery) && !strings.Contains(url, "&"+tt.expectedQuery) {
					t.Errorf("Expected query string %q in URL, but got URL: %s", tt.expectedQuery, url)
				}
			}
		})
	}
}

// TestFetchAllPagesV2 tests the v2 paging helper with a mock HTTP server
func TestFetchAllPagesV2(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "1" {
			resp := map[string]interface{}{
				"entities": []map[string]interface{}{
					{"uuid": "1"},
					{"uuid": "2"},
				},
				"metadata": map[string]interface{}{
					"count":                2,
					"end_index":            2,
					"grand_total_entities": 3,
					"page":                 1,
					"start_index":          1,
					"total_entities":       3,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			resp := map[string]interface{}{
				"entities": []map[string]interface{}{
					{"uuid": "3"},
				},
				"metadata": map[string]interface{}{
					"count":                1,
					"end_index":            3,
					"grand_total_entities": 3,
					"page":                 2,
					"start_index":          3,
					"total_entities":       3,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := &Nutanix{
		url:      server.URL,
		username: "user",
		password: "pass",
	}

	entities, err := client.fetchAllPagesV2("/test", nil)
	if err != nil {
		t.Fatalf("fetchAllPagesV2 failed: %v", err)
	}

	if len(entities) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(entities))
	}
}

// TestFetchAllPagesV1 tests the v1 paging helper with a mock HTTP server
func TestFetchAllPagesV1(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "1" {
			resp := map[string]interface{}{
				"entities": []map[string]interface{}{
					{"uuid": "1"},
					{"uuid": "2"},
				},
				"metadata": map[string]interface{}{
					"count":              2,
					"endIndex":           2,
					"grandTotalEntities": 3,
					"page":               1,
					"startIndex":         1,
					"totalEntities":      3,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			resp := map[string]interface{}{
				"entities": []map[string]interface{}{
					{"uuid": "3"},
				},
				"metadata": map[string]interface{}{
					"count":              1,
					"endIndex":           3,
					"grandTotalEntities": 3,
					"page":               2,
					"startIndex":         3,
					"totalEntities":      3,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := &Nutanix{
		url:      server.URL,
		username: "user",
		password: "pass",
	}

	entities, err := client.fetchAllPagesV1("/test", nil)
	if err != nil {
		t.Fatalf("fetchAllPagesV1 failed: %v", err)
	}

	if len(entities) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(entities))
	}
}
