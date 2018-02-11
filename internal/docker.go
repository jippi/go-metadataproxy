package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/fsouza/go-dockerclient"
	log "github.com/sirupsen/logrus"
)

var (
	dockerClient *docker.Client
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

func findDockerContainer(ip string) (*docker.Container, error) {
	containers, err := dockerClient.ListContainers(docker.ListContainersOptions{
		Filters: map[string][]string{
			"status": []string{"running"},
		},
	})

	if err != nil {
		return nil, err
	}

	for _, container := range containers {
		for name, network := range container.Networks.Networks {
			if network.IPAddress == ip {
				log.Info("Found container IP %s in %+v wihthin network %s", ip, container.Names, name)
				return dockerClient.InspectContainer(container.ID)
			}
		}
	}

	return nil, fmt.Errorf("Could not find any container with IP %s", ip)
}

func findDockerContainerIAMRole(container *docker.Container) (string, error) {
	for _, envPair := range container.Config.Env {
		chunks := strings.SplitN(envPair, "=", 2)

		if chunks[0] == "IAM_ROLE" {
			return chunks[1], nil
		}
	}

	if defaultRole := os.Getenv("DEFAULT_ROLE"); defaultRole != "" {
		log.Infof("Could not find IAM_ROLE in the container, returning DEFAULT_ROLE %s", defaultRole)
		return defaultRole, nil
	}

	return "", fmt.Errorf("Could not find IAM_ROLE in the container ENV config")
}
