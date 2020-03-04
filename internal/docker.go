package internal

import (
	"fmt"
	"os"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	log "github.com/sirupsen/logrus"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	dockerClient       *docker.Client
	defaultRole        = os.Getenv("DEFAULT_ROLE")
	copyDockerLabels   = strings.Split(os.Getenv("COPY_DOCKER_LABELS"), ",")
	copyDockerEnvs     = strings.Split(os.Getenv("COPY_DOCKER_ENV"), ",")
	copyRequestHeaders = strings.Split(os.Getenv("COPY_REQUEST_HEADERS"), ",")
	labelSeparator     = getenvDefault("LABEL_SEPARATOR", "_")
)

// ConfigureDocker will setup a docker client used during normal operations
func ConfigureDocker() {
	log.Info("Connecting to Docker daemon")

	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatalf("Could not create Docker client: %s", err.Error())
	}

	info, err := client.Info()
	if err != nil {
		log.Fatalf("Could not get Docker info: %s", err.Error())
	}

	log.Infof("Connected to Docker daemon: %s @ %s", info.Name, info.ServerVersion)
	dockerClient = client
}

func findDockerContainer(ip string, request *Request, parentSpan tracer.Span) (*docker.Container, error) {
	span := tracer.StartSpan("findDockerContainer", tracer.ChildOf(parentSpan.Context()), tracer.ServiceName("docker"))
	defer span.Finish()
	span.SetTag("docker.ip", ip)

	var container *docker.Container
	request.log.Infof("Looking up container info for %s in docker", ip)
	containers, err := dockerClient.ListContainers(docker.ListContainersOptions{All: true})
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, err
	}

	container, err = findContainerByIP(ip, request, containers, span)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, err
	}

	additionalLabels := make(map[string]string)
	if len(copyDockerLabels) > 0 {
		for _, label := range copyDockerLabels {
			if v, ok := container.Config.Labels[label]; ok {
				additionalLabels[labelName("container", label)] = v
			}
		}
	}

	if len(copyDockerEnvs) > 0 {
		for _, label := range copyDockerEnvs {
			if v, ok := findDockerContainerEnvValue(container, label); ok {
				additionalLabels[labelName("container", label)] = v
			}
		}
	}

	if len(additionalLabels) > 0 {
		request.setLabels(additionalLabels)
	}

	return container, nil
}

func findContainerByIP(ip string, request *Request, containers []docker.APIContainers, parentSpan tracer.Span) (*docker.Container, error) {
	span := tracer.StartSpan("findContainerByIP", tracer.ChildOf(parentSpan.Context()))
	defer span.Finish()
	span.SetTag("docker.ip", ip)

	for _, container := range containers {
		for name, network := range container.Networks.Networks {
			if network.IPAddress == ip {
				request.log.Infof("Found container IP '%s' in %+v within network '%s'", ip, container.Names, name)

				inspectedContainer, err := dockerClient.InspectContainer(container.ID)
				if err != nil {
					span.Finish(tracer.WithError(err))
					return nil, err
				}

				return inspectedContainer, nil
			}
		}
	}

	return nil, fmt.Errorf("Could not find any container with IP %s", ip)
}

func findDockerContainerIAMRole(container *docker.Container, request *Request) (string, error) {
	if v, ok := findDockerContainerEnvValue(container, "IAM_ROLE"); ok {
		return v, nil
	}

	if defaultRole != "" {
		request.log.Infof("Could not find IAM_ROLE in the container, returning DEFAULT_ROLE %s", defaultRole)
		return defaultRole, nil
	}

	return "", fmt.Errorf("Could not find IAM_ROLE in the container ENV config")
}

func findDockerContainerexternalID(container *docker.Container, request *Request) string {
	v, _ := findDockerContainerEnvValue(container, "IAM_EXTERNAL_ID")
	return v
}

func findDockerContainerEnvValue(container *docker.Container, key string) (string, bool) {
	for _, envPair := range container.Config.Env {
		chunks := strings.SplitN(envPair, "=", 2)

		if chunks[0] == key {
			return chunks[1], true
		}
	}

	return "", false
}

func labelName(prefix, label string) string {
	return fmt.Sprintf("%s%s%s", prefix, labelSeparator, strings.ToLower(label))
}

func getenvDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	return value
}
