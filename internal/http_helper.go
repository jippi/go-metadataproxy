package internal

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/cenkalti/backoff"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/gorilla/mux"
)

func remoteIP(addr string) string {
	return strings.Split(addr, ":")[0]
}

func findContainerRoleByAddress(addr string, request *Request) (*iam.Role, string, error) {
	var container *docker.Container

	// retry finding the Docker container since sometimes Docker doesn't actually list the container until its been
	// running for a while. This is a really simple and basic retry policy
	remoteIP := remoteIP(addr)

	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 5 * time.Second
	b.InitialInterval = 5 * time.Millisecond

	retryable := func() error {
		var err error
		container, err = findDockerContainer(remoteIP, request)
		return err
	}

	notify := func(err error, t time.Duration) {
		request.log.Errorf("%s in %d", err, t)
	}

	err := backoff.RetryNotify(retryable, b, notify)
	if err != nil {
		return nil, "", err
	}

	roleName, err := findDockerContainerIAMRole(container, request)
	if err != nil {
		return nil, "", err
	}

	role, err := readRoleFromAWS(roleName, request)
	if err != nil {
		return nil, "", err
	}

	externalID := findDockerContainerexternalID(container, request)

	return role, externalID, nil
}

func isCompatibleAPIVersion(r *http.Request) bool {
	vars := mux.Vars(r)
	return vars["api_version"] >= "2012-01-12"
}

func httpError(err error, w http.ResponseWriter, r *http.Request, request *Request) {
	request.log.Error(err)
	http.NotFound(w, r)
}

func sendJSONResponse(w http.ResponseWriter, response interface{}) {
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
