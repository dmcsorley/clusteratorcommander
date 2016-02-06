package main

import (
	"clusterator/libclusterator"
	"fmt"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"
	"github.com/docker/go-connections/nat"
	"github.com/docker/machine/commands/mcndirs"
	"github.com/docker/machine/libmachine"
	"log"
	"os"
	"strconv"
	"time"
)

const (
	CLUSTER_NAME = "barney"
	CONSUL_CONTAINER_NAME = "clusterator_consul"
	CONSUL_AMD64_IMAGE = "progrium/consul"
	SWARM_AGENT_CONTAINER_NAME = "clusterator_swarm_agent"
	SWARM_AMD64_IMAGE = "swarm"
	SWARM_MASTER_CONTAINER_NAME = "clusterator_swarm_master"
)

func standardHostConfig() *container.HostConfig {
	return &container.HostConfig{
		LogConfig: container.LogConfig{
			Type: "json-file",
			Config: map[string]string{
				"max-size": "10m",
				"max-file": "5",
			},
		},
		RestartPolicy: container.RestartPolicy{
			Name: "always",
		},
	}
}

func startConsul(conn libclusterator.DockerConnection, command *strslice.StrSlice) error {
	containerConfig := &container.Config{
		Image: CONSUL_AMD64_IMAGE,
		Cmd: command,
	}

	hostConfig := standardHostConfig()
	hostConfig.NetworkMode = "host"

	return conn.RunImage(containerConfig, hostConfig, CONSUL_CONTAINER_NAME)
}

func startSwarmAgent(conn libclusterator.DockerConnection) error {
	containerConfig := &container.Config{
		Image: SWARM_AMD64_IMAGE,
		Cmd: strslice.New("join", "--advertise", conn.GetDockerURL().GetHostPort(), conn.GetDiscoveryURL(CLUSTER_NAME)),
	}

	hostConfig := standardHostConfig()

	return conn.RunImage(containerConfig, hostConfig, SWARM_AGENT_CONTAINER_NAME)
}

func startSwarmMaster(conn libclusterator.DockerConnection) error {
	port := "3376"
	host := conn.GetDockerURL().GetHost()
	advertise := host + ":" + port

	containerConfig := &container.Config{
		Image: SWARM_AMD64_IMAGE,
		Cmd: strslice.New(
			"manage",
			"--replication",
			"--advertise",
			advertise,
			"--tlsverify",
			"--tlscacert=/certs/ca.pem",
			"--tlscert=/certs/server.pem",
			"--tlskey=/certs/server-key.pem",
			conn.GetDiscoveryURL(CLUSTER_NAME),
		),
	}

	hostConfig := standardHostConfig()
	hostConfig.Binds = []string{"/var/lib/boot2docker:/certs"}
	hostConfig.PortBindings = nat.PortMap{
		nat.Port("2375/tcp"): []nat.PortBinding{nat.PortBinding{HostIP:host, HostPort:port}},
	}

	return conn.RunImage(containerConfig, hostConfig, SWARM_MASTER_CONTAINER_NAME)
}

func startSwarm(conn libclusterator.DockerConnection) error {
	err := startSwarmAgent(conn)
	if err != nil {
		return err
	}

	err = startSwarmMaster(conn)
	if err != nil {
		return err
	}

	return conn.SaveSwarmConfig(CLUSTER_NAME)
}

func clusterCreate(api *libmachine.Client, hostnames []string) {
	// create the consul cluster first
	quorum := len(hostnames)/2 + 1
	connection := libclusterator.NewConnection(api, hostnames[0])

	err := startConsul(connection, strslice.New("-server", "-bind", connection.GetDockerURL().GetHost(), "-bootstrap-expect", strconv.Itoa(quorum)))
	if err != nil {
		log.Fatal(err)
	}

	consulJoinURL := connection.GetDockerURL().GetHost() + ":8301"

	connections := libclusterator.ForAllMachines(api, hostnames[1:], func(conn libclusterator.DockerConnection) {
		err := startConsul(conn, strslice.New("-server", "-bind", conn.GetDockerURL().GetHost(), "-join", consulJoinURL))
		if err != nil {
			log.Fatal(err)
		}
	})

	time.Sleep(500*time.Millisecond)

	connections = append(connections, connection)
	for _, conn := range connections {
		if err = startSwarm(conn); err != nil {
			log.Fatal(err)
		}
	}
}

func clusterDestroy(api *libmachine.Client, hostnames []string) {
	libclusterator.ForAllMachines(api, hostnames, func(conn libclusterator.DockerConnection) {
		conn.ForceRemoveContainers([]string{CONSUL_CONTAINER_NAME, SWARM_AGENT_CONTAINER_NAME, SWARM_MASTER_CONTAINER_NAME})
	})
}

func main() {
	api := libmachine.NewClient(mcndirs.GetBaseDir(), mcndirs.GetMachineCertDir())
	defer api.Close()

	command := os.Args[1]

	switch command {
	case "create": clusterCreate(api, os.Args[2:])
	case "destroy": clusterDestroy(api, os.Args[2:])
	default: fmt.Println("nope!")
	}
}

