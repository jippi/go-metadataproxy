package internal

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// handles: /{api_version}/meta-data/iam/info
// handles: /{api_version}/meta-data/iam/info/{junk}
func IamRoleInfoHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// ensure we got compatible api version
	if !isCompatibleApiVersion(r) {
		log.Info("Request is using too old version of meta-data API, passing through directly")
		PassthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := getIamRole(r)
	if err != nil {
		log.Error(err)
		http.NotFound(w, r)
		return
	}

	// assume the role
	assumeRole, err := assumeRoleFromAWS(*roleInfo.Role.Arn)
	if err != nil {
		log.Error(err)
		http.NotFound(w, r)
		return
	}

	// build response
	response := map[string]string{
		"Code":               "Success",
		"LastUpdated":        "now?", // TODO
		"InstanceProfileArn": *assumeRole.AssumedRoleUser.Arn,
		"InstanceProfileId":  *assumeRole.AssumedRoleUser.AssumedRoleId,
	}

	// send response
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handles: {api_version}/meta-data/iam/security-credentials/
func IamRoleNameHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// ensure we got compatible api version
	if !isCompatibleApiVersion(r) {
		log.Info("Request is using too old version of meta-data API, passing through directly")
		PassthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := getIamRole(r)
	if err != nil {
		log.Error(err)
		http.NotFound(w, r)
		return
	}

	// send the response
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(*roleInfo.Role.RoleName))
}

// handles: /{api_version}/meta-data/iam/security-credentials/{requested_role}
func IamStsCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s from %s", r.URL.String(), r.RemoteAddr)

	// ensure we got compatible api version
	if !isCompatibleApiVersion(r) {
		log.Info("Request is using too old version of meta-data API, passing through directly")
		PassthroughHandler(w, r)
		return
	}

	// read the role from AWS
	roleInfo, err := getIamRole(r)
	if err != nil {
		log.Error(err)
		http.NotFound(w, r)
		return
	}

	// verify the requested role match the container role
	vars := mux.Vars(r)
	if vars["requested_role"] != *roleInfo.Role.RoleName {
		log.Error("role names do not match")
		http.NotFound(w, r)
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
		"LastUpdated":     "now?", // TODO
		"Type":            "AWS-HMAC",
		"AccessKeyId":     *assumeRole.Credentials.AccessKeyId,
		"SecretAccessKey": *assumeRole.Credentials.SecretAccessKey,
		"Token":           *assumeRole.Credentials.SessionToken,
		"Expiration":      "now??", // TODO
	}

	// send response
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handles: /*
func PassthroughHandler(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Add("X-Proxy-By", "go-metadataproxy")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
