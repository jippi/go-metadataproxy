package internal

import (
	"os"

	gelf "github.com/seatgeek/logrus-gelf-formatter"
	log "github.com/sirupsen/logrus"
)

// ConfigureLogging will setup logging for the system
func ConfigureLogging() {
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		level, err := log.ParseLevel(level)
		if err != nil {
			log.Fatal(err)
		}
		log.SetLevel(level)
	}

	if format := os.Getenv("LOG_FORMAT"); format != "" {
		switch format {
		case "text":
			// the default
		case "json":
			log.SetFormatter(&log.JSONFormatter{})
		case "gelf":
			log.SetFormatter(&gelf.GelfFormatter{})
		default:
			log.Fatal("Unknown log_format (text, json or gelf)")
		}
	}
}
