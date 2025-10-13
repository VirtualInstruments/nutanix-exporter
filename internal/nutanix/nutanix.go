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
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
}

func (g *Nutanix) makeV1Request(reqType string, action string, params url.Values) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V1, reqType, action, RequestParams{params: params})
}

func (g *Nutanix) makeV2Request(reqType string, action string, params url.Values) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V2, reqType, action, RequestParams{params: params})
}

func (g *Nutanix) makeRequestWithParams(versionPath, reqType, action string, p RequestParams) (*http.Response, error) {
	_url := strings.Trim(g.url, "/")
	_url += "/PrismGateway/services/rest/" + versionPath
	_url += strings.Trim(action, "/") + "/"

	log.Debugf("URL: %s", _url)

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	var netClient = http.Client{
		Transport: tr,
		Timeout:   HTTP_TIMEOUT,
	}

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
		// heuristics for health
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			IncConnTimeout()
		} else if strings.Contains(strings.ToLower(err.Error()), "no such host") {
			IncDNSFailure()
		} else {
			IncException()
		}
		MarkCmdFailure(time.Since(start))
		return nil, err
	}

	if resp.StatusCode >= 400 {
		log.Errorf("error status from server; status=%v code=%v\n", resp.Status, resp.StatusCode)
		MarkCmdFailure(time.Since(start))
		return nil, fmt.Errorf("error status received")
	}

	MarkCmdSuccess(time.Since(start))
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
