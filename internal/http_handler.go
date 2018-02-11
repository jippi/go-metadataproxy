package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// StarServer will start the HTTP server (blocking)
func StarServer() {
	r := mux.NewRouter()
	r.HandleFunc("/{api_version}/meta-data/iam/info", roleInfoHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/info/{junk}", roleInfoHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/", roleNameHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/{requested_role}", credentialsHandler)
	r.PathPrefix("/").HandlerFunc(passthroughHandler)

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
func roleInfoHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		log.Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := findContainerRoleByAddress(r.RemoteAddr)
	if err != nil {
		httpError(err, w, r)
		return
	}

	// assume the role
	assumeRole, err := assumeRoleFromAWS(*roleInfo.Role.Arn)
	if err != nil {
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
}

// handles: {api_version}/meta-data/iam/security-credentials/
func roleNameHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		log.Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := findContainerRoleByAddress(r.RemoteAddr)
	if err != nil {
		httpError(err, w, r)
		return
	}

	// send the response
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(*roleInfo.Role.RoleName))
}

// handles: /{api_version}/meta-data/iam/security-credentials/{requested_role}
func credentialsHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// ensure we got compatible api version
	if !isCompatibleAPIVersion(r) {
		log.Info("Request is using too old version of meta-data API, passing through directly")
		passthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := findContainerRoleByAddress(r.RemoteAddr)
	if err != nil {
		httpError(err, w, r)
		return
	}

	// verify the requested role match the container role
	vars := mux.Vars(r)
	if vars["requested_role"] != *roleInfo.Role.RoleName {
		httpError(fmt.Errorf("Role names do not match"), w, r)
		return
	}

	// assume the container role
	assumeRole, err := assumeRoleFromAWS(*roleInfo.Role.Arn)
	if err != nil {
		log.Error(err)
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
}

// handles: /*
func passthroughHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	r.RequestURI = ""

	// ensure the chema and correct IP is set
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
		r.URL.Host = "169.254.169.254"
		r.Host = "169.254.169.254"
	}

	// create HTTP client
	client := &http.Client{}

	// use the incoming http request to construct upstream request
	resp, err := client.Do(r)
	if err != nil {
		http.Error(w, "Server Error", http.StatusInternalServerError)
		log.Fatal("ServeHTTP:", err)
	}
	defer resp.Body.Close()

	w.Header().Add("X-Powered-By", "go-metadataproxy")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
