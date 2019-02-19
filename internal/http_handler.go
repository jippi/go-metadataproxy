package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/newrelic/go-agent"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/security-credentials/{requested_role}", iamSecurityCredentialsForRole))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/security-credentials/{requested_role}/", iamSecurityCredentialsForRole))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/security-credentials", iamSecurityCredentialsName))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/meta-data/iam/security-credentials/", iamSecurityCredentialsName))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{api_version}/{rest:.*}", passthroughHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/metrics", metricsHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/favicon.ico", notFoundHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/{rest:.*}", passthroughHandler))
		r.HandleFunc(newrelic.WrapHandleFunc(app, "/", passthroughHandler))
	} else {
		r.HandleFunc("/{api_version}/meta-data/iam/info", iamInfoHandler)
		r.HandleFunc("/{api_version}/meta-data/iam/info/{junk}", iamInfoHandler)
		r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/{requested_role}", iamSecurityCredentialsForRole)
		r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/{requested_role}/", iamSecurityCredentialsForRole)
		r.HandleFunc("/{api_version}/meta-data/iam/security-credentials", iamSecurityCredentialsName)
		r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/", iamSecurityCredentialsName)
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
	vars := mux.Vars(r)

	request := NewRequest()
	request.setLabelsFromRequestHeader(r)
	request.setLabels(map[string]string{
		"aws_api_version": vars["api_version"],
		"handler_name":    "iam-info-handler",
		"remote_addr":     r.RemoteAddr,
		"request_path":    "/meta-data/iam/info",
	})
	request.log.Infof("Handling %s from %s", r.URL.String(), remoteIP(r.RemoteAddr))

	// publish specific go-metadataproxy headers
	request.setResponseHeaders(w)

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		request.log.Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := findContainerRoleByAddress(r.RemoteAddr, request)
	if err != nil {
		request.setLabels(map[string]string{
			"response_code":     "404",
			"error_description": "could_not_find_container",
		})
		request.incrCounterWithLabels([]string{"http_request"}, 1)

		httpError(err, w, r, request)
		return
	}

	// append role name to future telemetry
	request.setLabel("role_name", *roleInfo.RoleName)

	// assume the role
	assumeRole, err := assumeRoleFromAWS(*roleInfo.Arn, request)
	if err != nil {
		request.setLabels(map[string]string{
			"response_code":     "404",
			"error_description": "could_not_assume_role",
		})
		request.incrCounterWithLabels([]string{"http_request"}, 1)

		httpError(err, w, r, request)
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

	request.setLabel("response_code", "200")
	request.incrCounterWithLabels([]string{"http_request"}, 1)
}

// handles: /{api_version}/meta-data/iam/security-credentials/
func iamSecurityCredentialsName(w http.ResponseWriter, r *http.Request) {
	// setup basic telemetry
	vars := mux.Vars(r)

	request := NewRequest()
	request.setLabelsFromRequestHeader(r)
	request.setLabels(map[string]string{
		"aws_api_version": vars["api_version"],
		"handler_name":    "iam-security-credentials-name",
		"remote_addr":     remoteIP(r.RemoteAddr),
		"request_path":    "/meta-data/iam/security-credentials/",
	})
	request.log.Infof("Handling %s from %s", r.URL.String(), remoteIP(r.RemoteAddr))

	// publish specific go-metadataproxy headers
	request.setResponseHeaders(w)

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		request.log.Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := findContainerRoleByAddress(r.RemoteAddr, request)
	if err != nil {
		request.setLabels(map[string]string{
			"response_code":     "404",
			"error_description": "could_not_find_container",
		})
		request.incrCounterWithLabels([]string{"http_request"}, 1)

		httpError(err, w, r, request)
		return
	}

	// send the response
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(*roleInfo.RoleName))

	request.setLabel("response_code", "200")
	request.incrCounterWithLabels([]string{"http_request"}, 1)
}

// handles: /{api_version}/meta-data/iam/security-credentials/{requested_role}
func iamSecurityCredentialsForRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	request := NewRequest()
	request.setLabelsFromRequestHeader(r)
	request.setLabels(map[string]string{
		"aws_api_version": vars["api_version"],
		"handler_name":    "iam-security-crentials-for-role",
		"remote_addr":     remoteIP(r.RemoteAddr),
		"request_path":    "/meta-data/iam/security-credentials/{requested_role}",
		"requested_role":  vars["requested_role"],
	})
	request.log.Infof("Handling %s from %s", r.URL.String(), remoteIP(r.RemoteAddr))

	// publish specific go-metadataproxy headers
	request.setResponseHeaders(w)

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		request.log.Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := findContainerRoleByAddress(r.RemoteAddr, request)
	if err != nil {
		request.setLabels(map[string]string{
			"response_code":     "404",
			"error_description": "could_not_find_container",
		})
		request.incrCounterWithLabels([]string{"http_request"}, 1)

		httpError(err, w, r, request)
		return
	}

	// verify the requested role match the container role
	if vars["requested_role"] != *roleInfo.RoleName {
		request.setLabels(map[string]string{
			"response_code":     "404",
			"error_description": "role_names_do_not_match",
		})
		request.incrCounterWithLabels([]string{"http_request"}, 1)

		httpError(fmt.Errorf("Role names do not match (requested: '%s' vs container role: '%s')", vars["requested_role"], *roleInfo.RoleName), w, r, request)
		return
	}

	// assume the container role
	assumeRole, err := assumeRoleFromAWS(*roleInfo.Arn, request)
	if err != nil {
		request.setLabels(map[string]string{
			"response_code":     "404",
			"error_description": "could_not_assume_role",
		})
		request.incrCounterWithLabels([]string{"http_request"}, 1)
		request.log.Error(err)

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

	request.setLabel("response_code", "200")
	request.incrCounterWithLabels([]string{"http_request"}, 1)
}

// handles: /*
func passthroughHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	request := NewRequest()
	request.setLabelsFromRequestHeader(r)
	request.setLabels(map[string]string{
		"aws_api_version": vars["api_version"],
		"handler_name":    "passthrough",
		"remote_addr":     remoteIP(r.RemoteAddr),
		"request_path":    r.URL.String(),
	})
	request.log.Infof("Handling %s from %s", r.URL.String(), remoteIP(r.RemoteAddr))

	// publish specific go-metadataproxy headers
	request.setResponseHeaders(w)

	// try to enrich the telemetry with additional labels
	// if this fail, we will still proxy the request as-is
	findContainerRoleByAddress(r.RemoteAddr, request)

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
		request.setGaugeWithLabels([]string{"aws_response_time"}, float32(tp.Duration()))
		request.setGaugeWithLabels([]string{"aws_request_time"}, float32(tp.ReqDuration()))
		request.setGaugeWithLabels([]string{"aws_connection_time"}, float32(tp.ConnDuration()))
	}()

	// use the incoming http request to construct upstream request
	resp, err := client.Do(r)
	if err != nil {
		request.setLabels(map[string]string{
			"response_code":     "404",
			"error_description": "could_not_assume_role",
		})
		request.incrCounterWithLabels([]string{"http_request"}, 1)

		httpError(fmt.Errorf("Could not proxy request: %s", err), w, r, request)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	request.setLabel("response_code", fmt.Sprintf("%v", resp.StatusCode))
	request.incrCounterWithLabels([]string{"http_request"}, 1)
}

// handles: /metrics
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	request := NewRequest()
	request.setLabelsFromRequestHeader(r)
	request.setLabels(map[string]string{
		"handler_name": "metrics",
		"remote_addr":  remoteIP(r.RemoteAddr),
		"request_path": "/metrics",
	})
	request.incrCounterWithLabels([]string{"http_request"}, 1)

	// publish specific go-metadataproxy headers
	request.setResponseHeaders(w)

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
		request.log.Error(err)
		return
	}

	sendJSONResponse(w, data)
}

// handles: /favicon.ico
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
