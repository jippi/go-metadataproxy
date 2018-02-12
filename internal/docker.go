package internal

import (
	"fmt"
	"os"
	"strings"

	metrics "github.com/armon/go-metrics"
	"github.com/fsouza/go-dockerclient"
	log "github.com/sirupsen/logrus"
)

var (
	dockerClient     *docker.Client
	defaultRole      = os.Getenv("DEFAULT_ROLE")
	copyDockerLabels = strings.Split(os.Getenv("COPY_DOCKER_LABELS"), ",")
	copyDockerEnvs   = strings.Split(os.Getenv("COPY_DOCKER_ENV"), ",")
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

func findDockerContainer(ip string, labels []metrics.Label) (*docker.Container, []metrics.Label, error) {
	containers, err := dockerClient.ListContainers(docker.ListContainersOptions{
		Filters: map[string][]string{
			"status": []string{"running"},
		},
	})

	if err != nil {
		return nil, labels, err
	}

	for _, container := range containers {
		for name, network := range container.Networks.Networks {
			if network.IPAddress == ip {
				log.Info("Found container IP %s in %+v wihthin network %s", ip, container.Names, name)

				inspectedContainer, err := dockerClient.InspectContainer(container.ID)
				if err != nil {
					return nil, labels, err
				}

				if len(copyDockerLabels) > 0 {
					for _, label := range copyDockerLabels {
						if v, ok := inspectedContainer.Config.Labels[label]; ok {
							labels = append(labels, metrics.Label{Name: strings.ToLower(label), Value: v})
						}
					}
				}

				if len(copyDockerEnvs) > 0 {
					for _, label := range copyDockerEnvs {
						if v, ok := findDockerContainerEnvValue(inspectedContainer, label); ok {
							labels = append(labels, metrics.Label{Name: strings.ToLower(label), Value: v})
						}
					}
				}

				return inspectedContainer, labels, nil
			}
		}
	}

	return nil, labels, fmt.Errorf("Could not find any container with IP %s", ip)
}

func findDockerContainerIAMRole(container *docker.Container) (string, error) {
	if v, ok := findDockerContainerEnvValue(container, "IAM_ROLE"); ok {
		return v, nil
	}

	if defaultRole != "" {
		log.Infof("Could not find IAM_ROLE in the container, returning DEFAULT_ROLE %s", defaultRole)
		return defaultRole, nil
	}

	return "", fmt.Errorf("Could not find IAM_ROLE in the container ENV config")
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
