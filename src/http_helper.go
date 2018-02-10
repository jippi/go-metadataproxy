package main

import (
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/gorilla/mux"
)

func getIamRole(r *http.Request) (*iam.GetRoleOutput, error) {
	remoteIP := remoteIP(r.RemoteAddr)

	container, err := findDockerContainer(remoteIP)
	if err != nil {
		return nil, err
	}

	roleName, err := findDockerContainerIamRole(container)
	if err != nil {
		return nil, err
	}

	role, err := readRoleFromAWS(roleName)
	if err != nil {
		return nil, err
	}

	return role, nil
}

func remoteIP(addr string) string {
	return strings.Split(addr, ":")[0]
}

func isCompatibleApiVersion(r *http.Request) bool {
	vars := mux.Vars(r)
	return vars["api_version"] >= "2012-01-12"
}
