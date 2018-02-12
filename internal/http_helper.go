package internal

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func remoteIP(addr string) string {
	return strings.Split(addr, ":")[0]
}

func findContainerRoleByAddress(addr string, labels []metrics.Label) (*iam.Role, []metrics.Label, error) {
	remoteIP := remoteIP(addr)

	container, labels, err := findDockerContainer(remoteIP, labels)
	if err != nil {
		return nil, labels, err
	}

	roleName, err := findDockerContainerIAMRole(container)
	if err != nil {
		return nil, labels, err
	}

	role, err := readRoleFromAWS(roleName)
	if err != nil {
		return nil, labels, err
	}

	return role, labels, nil
}

func isCompatibleAPIVersion(r *http.Request) bool {
	vars := mux.Vars(r)
	return vars["api_version"] >= "2012-01-12"
}

func httpError(err error, w http.ResponseWriter, r *http.Request) {
	log.Error(err)
	http.NotFound(w, r)
}

func sendJSONResponse(w http.ResponseWriter, response interface{}) {
	w.Header().Add("X-Powered-By", "go-metadataproxy")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(response)
}

type customTransport struct {
	rtp       http.RoundTripper
	dialer    *net.Dialer
	connStart time.Time
	connEnd   time.Time
	reqStart  time.Time
	reqEnd    time.Time
}

func newTransport() *customTransport {
	tr := &customTransport{
		dialer: &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
		},
	}

	tr.rtp = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		Dial:                tr.dial,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return tr
}

func (tr *customTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	tr.reqStart = time.Now()
	resp, err := tr.rtp.RoundTrip(r)
	tr.reqEnd = time.Now()
	return resp, err
}

func (tr *customTransport) dial(network, addr string) (net.Conn, error) {
	tr.connStart = time.Now()
	cn, err := tr.dialer.Dial(network, addr)
	tr.connEnd = time.Now()

	return cn, err
}

func (tr *customTransport) ReqDuration() time.Duration {
	return tr.Duration() - tr.ConnDuration()
}

func (tr *customTransport) ConnDuration() time.Duration {
	return tr.connEnd.Sub(tr.connStart)
}

func (tr *customTransport) Duration() time.Duration {
	return tr.reqEnd.Sub(tr.reqStart)
}
