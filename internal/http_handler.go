package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/armon/go-metrics"
	"github.com/gorilla/mux"
	"github.com/newrelic/go-agent"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const (
	telemetryPrefix = "metadataproxy"
)

// StarServer will start the HTTP server (blocking)
func StarServer() {
	r := mux.NewRouter()

	newrelicAppName := os.Getenv("NEWRELIC_APP_NAME")
	newrelicLicense := os.Getenv("NEWRELIC_LICENSE")

	if newrelicAppName != "" && newrelicLicense != "" {
		config := newrelic.NewConfig(newrelicAppName, newrelicLicense)
		app, err := newrelic.NewApplication(config)
		if err != nil {
			log.Fatal(err)
		}

		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/info", iamInfoHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/info/{junk}", iamInfoHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/security-credentials/", iamSecurityCredentialsName))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/security-credentials/{requested_role}", iamSecurityCredentialsForRole))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/{rest:.*}", passthroughHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/metrics", metricsHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/favicon.ico", notFoundHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{rest:.*}", passthroughHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/", passthroughHandler))
	} else {
		r.HandleFunc("/{api_version}/meta-data/iam/info", iamInfoHandler)
		r.HandleFunc("/{api_version}/meta-data/iam/info/{junk}", iamInfoHandler)
		r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/", iamSecurityCredentialsName)
		r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/{requested_role}", iamSecurityCredentialsForRole)
		r.HandleFunc("/{api_version}/{rest:.*}", passthroughHandler)
		r.HandleFunc("/metrics", metricsHandler)
		r.HandleFunc("/favicon.ico", notFoundHandler)
		r.HandleFunc("/{rest:.*}", passthroughHandler)
		r.HandleFunc("/", passthroughHandler)
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	addr := fmt.Sprintf("%s:%s", host, port)

	log.Infof("Starting server at %s", addr)

	srv := &http.Server{
		Handler:      r,
		Addr:         addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}

// handles: /{api_version}/meta-data/iam/info
// handles: /{api_version}/meta-data/iam/info/{junk}
func iamInfoHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// setup basic telemetry
	vars := mux.Vars(r)
	labels := []metrics.Label{
		metrics.Label{Name: "api_version", Value: vars["api_version"]},
		metrics.Label{Name: "request_path", Value: "/meta-data/iam/info"},
		metrics.Label{Name: "handler_name", Value: "iam-info-handler"},
	}

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		logWithLabels(labels).Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, labels, err := findContainerRoleByAddress(r.RemoteAddr, labels)
	if err != nil {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "could_not_find_container"})
		metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)

		httpError(err, w, r)
		return
	}

	// append role name to future telemetry
	labels = append(labels, metrics.Label{Name: "role_name", Value: *roleInfo.RoleName})

	// assume the role
	assumeRole, labels, err := assumeRoleFromAWS(*roleInfo.Arn, labels)
	if err != nil {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "could_not_assume_role"})
		metrics.IncrCounterWithLabels([]string{}, 1, labels)

		httpError(err, w, r)
		return
	}

	// build response
	response := map[string]string{
		"Code":               "Success",
		"LastUpdated":        assumeRole.Credentials.Expiration.Add(-1 * time.Hour).Format(awsTimeLayoutResponse),
		"InstanceProfileArn": *assumeRole.AssumedRoleUser.Arn,
		"InstanceProfileId":  *assumeRole.AssumedRoleUser.AssumedRoleId,
	}

	sendJSONResponse(w, response)

	labels = append(labels, metrics.Label{Name: "response_code", Value: "200"})
	metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)
}

// handles: /{api_version}/meta-data/iam/security-credentials/
func iamSecurityCredentialsName(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// setup basic telemetry
	vars := mux.Vars(r)
	labels := []metrics.Label{
		metrics.Label{Name: "api_version", Value: vars["api_version"]},
		metrics.Label{Name: "request_path", Value: "/meta-data/iam/security-credentials/"},
		metrics.Label{Name: "handler_name", Value: "iam-security-credentials-name"},
	}

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		logWithLabels(labels).Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, labels, err := findContainerRoleByAddress(r.RemoteAddr, labels)
	if err != nil {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "could_not_find_container"})
		metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)

		httpError(err, w, r)
		return
	}

	// send the response
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(*roleInfo.RoleName))

	labels = append(labels, metrics.Label{Name: "response_code", Value: "200"})
	metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)
}

// handles: /{api_version}/meta-data/iam/security-credentials/{requested_role}
func iamSecurityCredentialsForRole(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// setup basic telemetry
	vars := mux.Vars(r)
	labels := []metrics.Label{
		metrics.Label{Name: "api_version", Value: vars["api_version"]},
		metrics.Label{Name: "request_path", Value: "/meta-data/iam/security-credentials/{requested_role}"},
		metrics.Label{Name: "requested_role", Value: vars["requested_role"]},
		metrics.Label{Name: "handler_name", Value: "iam-security-crentials-for-role"},
	}

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		logWithLabels(labels).Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, labels, err := findContainerRoleByAddress(r.RemoteAddr, labels)
	if err != nil {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "could_not_find_container"})
		metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)

		httpError(err, w, r)
		return
	}

	// verify the requested role match the container role
	if vars["requested_role"] != *roleInfo.RoleName {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "role_names_do_not_match"})
		metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)

		httpError(fmt.Errorf("Role names do not match"), w, r)
		return
	}

	// assume the container role
	assumeRole, labels, err := assumeRoleFromAWS(*roleInfo.Arn, labels)
	if err != nil {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "could_not_assume_role"})
		metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)

		logWithLabels(labels).Error(err)
		http.NotFound(w, r)
		return
	}

	// build response
	response := map[string]string{
		"Code":            "Success",
		"LastUpdated":     assumeRole.Credentials.Expiration.Add(-1 * time.Hour).Format(awsTimeLayoutResponse),
		"Type":            "AWS-HMAC",
		"AccessKeyId":     *assumeRole.Credentials.AccessKeyId,
		"SecretAccessKey": *assumeRole.Credentials.SecretAccessKey,
		"Token":           *assumeRole.Credentials.SessionToken,
		"Expiration":      assumeRole.Credentials.Expiration.Format(awsTimeLayoutResponse),
	}

	// send response
	sendJSONResponse(w, response)

	labels = append(labels, metrics.Label{Name: "response_code", Value: "200"})
	metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)
}

// handles: /*
func passthroughHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// setup basic telemetry
	vars := mux.Vars(r)
	labels := []metrics.Label{
		metrics.Label{Name: "api_version", Value: vars["api_version"]},
		metrics.Label{Name: "request_path", Value: r.URL.String()},
		metrics.Label{Name: "handler_name", Value: "passthrough"},
	}

	// read the role from AWS
	_, labels, err := findContainerRoleByAddress(r.RemoteAddr, labels)
	if err != nil {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "could_not_find_container"})
		metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)

		httpError(err, w, r)
		return
	}

	r.RequestURI = ""

	// ensure the chema and correct IP is set
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
		r.URL.Host = "169.254.169.254"
		r.Host = "169.254.169.254"
	}

	// create HTTP client
	tp := newTransport()
	client := &http.Client{Transport: tp}
	defer func() {
		metrics.SetGaugeWithLabels([]string{telemetryPrefix, "aws_response_time"}, float32(tp.Duration()), labels)
		metrics.SetGaugeWithLabels([]string{telemetryPrefix, "aws_request_time"}, float32(tp.ReqDuration()), labels)
		metrics.SetGaugeWithLabels([]string{telemetryPrefix, "aws_connection_time"}, float32(tp.ConnDuration()), labels)
	}()

	// use the incoming http request to construct upstream request
	resp, err := client.Do(r)
	if err != nil {
		labels = append(labels, metrics.Label{Name: "response_code", Value: "404"})
		labels = append(labels, metrics.Label{Name: "error_description", Value: "could_not_assume_role"})
		metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)

		httpError(fmt.Errorf("Could not proxy request: %s", err), w, r)
		return
	}
	defer resp.Body.Close()

	w.Header().Add("X-Powered-By", "go-metadataproxy")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	labels = append(labels, metrics.Label{Name: "response_code", Value: fmt.Sprintf("%v", resp.StatusCode)})
	metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, labels)
}

// handles: /metrics
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics.IncrCounterWithLabels([]string{telemetryPrefix, "http_request"}, 1, []metrics.Label{
		metrics.Label{Name: "request_path", Value: "/metrics"},
		metrics.Label{Name: "handler_name", Value: "metrics"},
	})

	if os.Getenv("ENABLE_PROMETHEUS") != "" {
		handlerOptions := promhttp.HandlerOpts{
			ErrorLog:           log.New(),
			ErrorHandling:      promhttp.ContinueOnError,
			DisableCompression: true,
		}

		handler := promhttp.HandlerFor(prometheus.DefaultGatherer, handlerOptions)
		handler.ServeHTTP(w, r)
		return
	}

	data, err := telemetry.DisplayMetrics(w, r)
	if err != nil {
		log.Error(err)
		return
	}

	sendJSONResponse(w, data)
}

// handles: /favicon.ico
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
