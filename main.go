package main

import (
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/{api_version}/meta-data/iam/info", passthroughHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/info/{junk}", passthroughHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/", passthroughHandler)
	r.HandleFunc("/{api_version}/meta-data/iam/security-credentials/{requested_role}", passthroughHandler)
	r.HandleFunc("/{path}", passthroughHandler)
	r.HandleFunc("/", passthroughHandler)

	log.Infof("Starting server at 0.0.0.0:3001")

	srv := &http.Server{
		Handler:      r,
		Addr:         "0.0.0.0:3001",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	srv.ListenAndServe()
}

func passthroughHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Handling %s", r.URL.String())

	r.RequestURI = ""

	// local debugging
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
		r.URL.Host = "expressional.com"
		r.Host = "expressional.com"
	}

	client := &http.Client{}
	resp, err := client.Do(r)
	if err != nil {
		http.Error(w, "Server Error", http.StatusInternalServerError)
		log.Fatal("ServeHTTP:", err)
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
