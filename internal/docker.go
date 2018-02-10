package internal

import (
	"fmt"
	"strings"

	"github.com/fsouza/go-dockerclient"
	log "github.com/sirupsen/logrus"
)

var (
	dockerClient *docker.Client
)

func ConfigureDocker() {
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
				log.Infof("found container ip %s in %+v in network %s", ip, container.Names, name)
				return dockerClient.InspectContainer(container.ID)
			}
		}
	}

	return nil, fmt.Errorf("Could not find any container with IP %s", ip)
}

func findDockerContainerIamRole(container *docker.Container) (string, error) {
	for _, envPair := range container.Config.Env {
		chunks := strings.SplitN(envPair, "=", 2)
		log.Debugf("k=%s v=%s", chunks[0], chunks[1])

		if chunks[0] == "IAM_ROLE" {
			return chunks[1], nil
		}
	}

	return "", fmt.Errorf("Could not find IAM_ROLE in containers ENV config")
}
