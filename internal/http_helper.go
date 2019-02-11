package internal

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const (
	retryCount = 5
	retrySleep = 5 * time.Millisecond
)

func remoteIP(addr string) string {
	return strings.Split(addr, ":")[0]
}

func findContainerRoleByAddress(addr string, labels []metrics.Label) (*iam.Role, []metrics.Label, error) {
	var container *docker.Container

	// retry finding the Docker container since sometimes Docker doesn't actually list the container until its been
	// running for a while. This is a really simple and basic retry policy
	var err error
	remoteIP := remoteIP(addr)
	for i := 1; i <= retryCount; i++ {
		container, labels, err = findDockerContainer(remoteIP, labels)
		// if we got no errors, just break the loop and keep moving forward
		if err == nil {
			break
		}

		// if we got an error, log that and take a quick nap
		logWithLabels(labels).Errorf("Could not find Docker container with remote IP %s (retry %d out of %d)", remoteIP, i, retryCount)
		time.Sleep(retrySleep)
	}

	// check if we got no errors from the "findDockerContainer" innerloop above
	if err != nil {
		return nil, labels, err
	}

	roleName, err := findDockerContainerIAMRole(container)
	if err != nil {
		return nil, labels, err
	}

	role, labels, err := readRoleFromAWS(roleName, labels)
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
	w.Header().Set("X-Powered-By", "go-metadataproxy")
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
