package nutanix

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// V2ResponseMetadata represents the metadata block returned by Nutanix v2 APIs
type V2ResponseMetadata struct {
	Count          int    `json:"count"`
	EndIndex       int    `json:"end_index"`
	FilterCriteria string `json:"filter_criteria"`
	GrandTotal     int    `json:"grand_total_entities"`
	Page           int    `json:"page"`
	SortCriteria   string `json:"sort_criteria"`
	StartIndex     int    `json:"start_index"`
	TotalEntities  int    `json:"total_entities"`
}

// V1ResponseMetadata represents the metadata block returned by Nutanix v1 APIs
type V1ResponseMetadata struct {
	Count          int    `json:"count"`
	EndIndex       int    `json:"endIndex"`
	FilterCriteria string `json:"filterCriteria"`
	GrandTotal     int    `json:"grandTotalEntities"`
	Page           int    `json:"page"`
	SortCriteria   string `json:"sortCriteria"`
	StartIndex     int    `json:"startIndex"`
	TotalEntities  int    `json:"totalEntities"`
}

// fetchAllPages is a unified helper that defaults to v2 paging
func (g *Nutanix) fetchAllPages(action string, baseParams url.Values) ([]interface{}, error) {
	return g.fetchAllPagesV2(action, baseParams)
}

// fetchAllPagesV2 is a generic helper to retrieve all pages from a v2 API endpoint
func (g *Nutanix) fetchAllPagesV2(action string, baseParams url.Values) ([]interface{}, error) {
	if baseParams == nil {
		baseParams = url.Values{}
	}
	// default count = 100
	if baseParams.Get("count") == "" {
		baseParams.Set("count", "100")
	}

	var allEntities []interface{}
	page := 1
	for {
		baseParams.Set("page", fmt.Sprintf("%d", page))
		resp, err := g.makeV2Request("GET", action, baseParams)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		entitiesRaw, ok := result["entities"]
		if !ok {
			break
		}
		entities, ok := entitiesRaw.([]interface{})
		if !ok {
			break
		}
		for _, e := range entities {
			allEntities = append(allEntities, e)
		}

		// parse metadata
		metaRaw, ok := result["metadata"]
		if !ok {
			break
		}
		metaBytes, _ := json.Marshal(metaRaw)
		var meta V2ResponseMetadata
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			return nil, err
		}

		if meta.EndIndex >= meta.GrandTotal {
			break
		}
		page++
	}

	return allEntities, nil
}

// fetchAllPagesV1 is a generic helper to retrieve all pages from a v1 API endpoint
func (g *Nutanix) fetchAllPagesV1(action string, baseParams url.Values) ([]interface{}, error) {
	if baseParams == nil {
		baseParams = url.Values{}
	}
	// default count = 100
	if baseParams.Get("count") == "" {
		baseParams.Set("count", "100")
	}

	var allEntities []interface{}
	page := 1
	for {
		baseParams.Set("page", fmt.Sprintf("%d", page))
		resp, err := g.makeV1Request("GET", action, baseParams)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		entitiesRaw, ok := result["entities"]
		if !ok {
			break
		}
		entities, ok := entitiesRaw.([]interface{})
		if !ok {
			break
		}
		for _, e := range entities {
			allEntities = append(allEntities, e)
		}

		// parse metadata
		metaRaw, ok := result["metadata"]
		if !ok {
			break
		}
		metaBytes, _ := json.Marshal(metaRaw)
		var meta V1ResponseMetadata
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			return nil, err
		}

		if meta.EndIndex >= meta.GrandTotal {
			break
		}
		page++
	}

	return allEntities, nil
}
