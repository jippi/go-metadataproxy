module github.com/jippi/go-metadataproxy

go 1.16

require (
	github.com/armon/go-metrics v0.3.8
	github.com/aws/aws-sdk-go-v2 v0.19.0
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/fsouza/go-dockerclient v1.7.4
	github.com/gorilla/mux v1.8.0
	github.com/newrelic/go-agent/v3 v3.15.0
	github.com/newrelic/go-agent/v3/integrations/nrgorilla v1.1.0
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/prometheus/client_golang v1.7.1
	github.com/satori/go.uuid v1.2.0
	github.com/seatgeek/logrus-gelf-formatter v0.0.0-20180829220724-ce23ecb3f367
	github.com/sirupsen/logrus v1.8.1
	github.com/tinylib/msgp v1.1.5 // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.32.0
)
