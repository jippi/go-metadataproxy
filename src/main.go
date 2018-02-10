package main

import (
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/fsouza/go-dockerclient"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Configure docker
	log.Info("Connecting to Docker Daemon")
	client, err := docker.NewClient("unix:///var/run/docker.sock")
	if err != nil {
		log.Fatalf("Could not create docker client: %s", err.Error())
	}

	info, err := client.Info()
	if err != nil {
		log.Fatalf("Could not get docker info: %s", err.Error())
	}
	log.Infof("Connected to docker daemon: %s @ %s", info.Name, info.ServerVersion)
	dockerClient = client

	// Configure AWS
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		log.Fatalf("unable to load AWS SDK config, " + err.Error())
	}
	iamService = iam.New(cfg)
	stsService = sts.New(cfg)

	// Configure HTTP handler
	r := mux.NewRouter()
	r.HandleFunc("/{api_version}/meta-data/iam/info", iamRoleInfoHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/info/{junk}", iamRoleInfoHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/", iamRoleNameHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/{requested_role}", iamStsCredentialsHandler)
	r.PathPrefix("/").HandlerFunc(passthroughHandler)

	log.Infof("Starting server at 0.0.0.0:8000")

	srv := &http.Server{
		Handler:      r,
		Addr:         "0.0.0.0:8000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
