package internal

import (
	"net/http"

	metrics "github.com/armon/go-metrics"
	"github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

const (
	telemetryPrefix = "metadataproxy"
)

type Request struct {
	id            string
	log           *logrus.Entry
	metricsLabels []metrics.Label
	loggingLabels logrus.Fields
}

func NewRequest() *Request {
	id := uuid.NewV4()

	return &Request{
		id:  id.String(),
		log: logrus.WithField("request_id", id.String()),
	}
}

func (r *Request) setLabel(key, value string) {
	r.setLabels(map[string]string{key: value})
}

func (r *Request) setLabels(pairs map[string]string) {
	for key, value := range pairs {
		r.metricsLabels = append(r.metricsLabels, metrics.Label{Name: key, Value: value})
		r.loggingLabels[key] = value
	}

	r.log = r.log.WithFields(r.loggingLabels)
}

func (r *Request) incrCounterWithLabels(path []string, val float32) {
	path = append([]string{telemetryPrefix}, path...)
	metrics.IncrCounterWithLabels(path, val, r.metricsLabels)
}

func (r *Request) setGaugeWithLabels(path []string, val float32) {
	path = append([]string{telemetryPrefix}, path...)
	metrics.SetGaugeWithLabels(path, val, r.metricsLabels)
}

func (r *Request) setResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Powered-By", "go-metadataproxy")
	w.Header().Set("X-Request-ID", r.id)
}
