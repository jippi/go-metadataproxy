package main

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/jippi/go-metadataproxy/internal"
	log "github.com/sirupsen/logrus"
)

func main() {
	internal.ConfigureDocker()
	internal.ConfigureAWS()

	// Configure HTTP handler
	r := mux.NewRouter()
	r.HandleFunc("/{api_version}/meta-data/iam/info", internal.IamRoleInfoHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/info/{junk}", internal.IamRoleInfoHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/", internal.IamRoleNameHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/{requested_role}", internal.IamStsCredentialsHandler)
	r.PathPrefix("/").HandlerFunc(internal.PassthroughHandler)

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
