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
	PRISM_API_PATH_VERSION_V1 = "v1/"
	PRISM_API_PATH_VERSION_V2 = "v2.0/"
	HTTP_TIMEOUT              = 10 * time.Second
)

type RequestParams struct {
	body, header string
	params       url.Values
}

type Nutanix struct {
	url      string
	username string
	password string
}

func (g *Nutanix) makeV1Request(reqType string, action string) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V1, reqType, action, RequestParams{})
}

func (g *Nutanix) makeV2Request(reqType string, action string) (*http.Response, error) {
	return g.makeRequestWithParams(PRISM_API_PATH_VERSION_V2, reqType, action, RequestParams{})
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

	_url += "?" + p.params.Encode()

	req, err := http.NewRequest(reqType, _url, strings.NewReader(body))
	if err != nil {
		log.Errorf("failed to create request; error=%v\n", err)
		return nil, err
	}
	//req.Header.Set("Content-Type", "text/JSON")

	req.SetBasicAuth(g.username, g.password)

	resp, err := netClient.Do(req)
	if err != nil {
		log.Errorf("failed to execute request; error=%v\n", err)
		return nil, err
	}
	if resp.StatusCode >= 400 {
		log.Errorf("error status from server; status=%v code=%v\n", resp.Status, resp.StatusCode)
		return nil, fmt.Errorf("error status received")
	}

	return resp, nil
}

func NewNutanix(url string, username string, password string) *Nutanix {
	//	log.SetOutput(os.Stdout)
	//	log.SetPrefix("Nutanix Logger")

	return &Nutanix{
		url:      url,
		username: username,
		password: password,
	}
}
