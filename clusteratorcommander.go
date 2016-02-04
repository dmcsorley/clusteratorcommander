package main

import (
	"clusterator/libclusterator"
	"encoding/json"
	"fmt"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"
	"github.com/docker/machine/commands/mcndirs"
	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/host"
	"log"
	"os"
	"strconv"
)

const (
	CONSUL_CONTAINER_NAME = "clusterator_consul"
	CONSUL_AMD64_IMAGE = "progrium/consul"
)

func rewriteConfig(api *libmachine.Client, hostname string) {
	host := libclusterator.LoadHost(api, hostname)
	ip, _ := host.Driver.GetIP()

	swarmopts := host.HostOptions.SwarmOptions
	swarmopts.IsSwarm = true
	swarmopts.Master = true
	swarmopts.Discovery = fmt.Sprintf("consul://%s:8500/barney", ip)

	api.Save(host)
}

func dmIP(api *libmachine.Client, hostnames []string) {
	libclusterator.ForAllHosts(api, hostnames, func(host *host.Host) {
		ip, _ := host.Driver.GetIP()
		fmt.Println(host.Name, ip)
	})
}

func printJson(api *libmachine.Client, hostname string) {
	host := libclusterator.LoadHost(api, hostname)
	prettyJSON, err := json.MarshalIndent(host, "", "    ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(prettyJSON))
}

func dmConfig(api *libmachine.Client, hostname string) {
	host := libclusterator.LoadHost(api, hostname)
	dockerHost, authOptions := libclusterator.GetHostOptions(host, true)

	fmt.Printf("--tlsverify\n--tlscacert=%q\n--tlscert=%q\n--tlskey=%q\n-H=%s\n",
		authOptions.CaCertPath, authOptions.ClientCertPath, authOptions.ClientKeyPath, dockerHost)
}

func dPs(api *libmachine.Client, hostname string) {
	host := libclusterator.LoadHost(api, hostname)
	dockerHost, authOptions := libclusterator.GetHostOptions(host, true)
	cli := libclusterator.CreateClient(dockerHost, authOptions)

	containers, err := cli.ContainerList(types.ContainerListOptions{All: true})
	if err != nil {
		panic(err)
	}

	for _, c := range containers {
		fmt.Println(c.ID)
	}
}

func dmStart(api *libmachine.Client, hostnames []string) {
	libclusterator.ForAllHosts(api, hostnames, func(host *host.Host) {
		host.Start()
	})
}

func startConsul(cli *client.Client, command *strslice.StrSlice) error {
	containerConfig := &container.Config{
		Image: CONSUL_AMD64_IMAGE,
		Cmd: command,
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "host",
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

	return libclusterator.RunImage(cli, containerConfig, hostConfig, CONSUL_CONTAINER_NAME)
}

func startFirstMaster(api *libmachine.Client, hostname string, quorum int) (string, error) {
	fmt.Println(hostname)
	host := libclusterator.LoadHost(api, hostname)
	ip, _ := host.Driver.GetIP()
	dockerHost, authOptions := libclusterator.GetHostOptions(host, false)
	cli := libclusterator.CreateClient(dockerHost, authOptions)
	err := startConsul(cli, strslice.New("-server", "-bind", ip, "-bootstrap-expect", strconv.Itoa(quorum)))
	if err != nil {
		return "", err
	}

	return ip, nil
}

func startOtherMaster(api *libmachine.Client, host *host.Host, masterConnection string) error {
	fmt.Println(host.Name)
	ip, _ := host.Driver.GetIP()
	dockerHost, authOptions := libclusterator.GetHostOptions(host, false)
	cli := libclusterator.CreateClient(dockerHost, authOptions)

	err := startConsul(cli, strslice.New("-server", "-bind", ip, "-join", masterConnection))
	return err
}

func clusterStart(api *libmachine.Client, hostnames []string) {
	quorum := len(hostnames)/2 + 1
	masterIP, err := startFirstMaster(api, hostnames[0], quorum)
	if err != nil {
		fmt.Println(err);
		return
	}

	masterConnection := masterIP + ":8301"

	libclusterator.ForAllHosts(api, hostnames[1:], func(host *host.Host) {
		err := startOtherMaster(api, host, masterConnection)
		if err != nil {
			fmt.Println(err)
		}
	})
}

func clusterDestroy(api *libmachine.Client, hostnames []string) {
	libclusterator.ForAllHosts(api, hostnames, func(host *host.Host) {
		fmt.Println(host.Name)
		dockerHost, authOptions := libclusterator.GetHostOptions(host, false)
		cli := libclusterator.CreateClient(dockerHost, authOptions)
		libclusterator.ForceRemoveContainer(cli, []string{CONSUL_CONTAINER_NAME})
	})
}

func url(api *libmachine.Client, hostnames []string) {
	libclusterator.ForAllHosts(api, hostnames, func(host *host.Host) {
		dockerHost, _ := libclusterator.GetHostOptions(host, false)
		fmt.Println(dockerHost)
	})
}

func main() {
	api := libmachine.NewClient(mcndirs.GetBaseDir(), mcndirs.GetMachineCertDir())
	defer api.Close()

	command := os.Args[1]

	switch command {
	case "ip": dmIP(api, os.Args[2:])
	case "json": printJson(api, os.Args[2])
	case "rewrite": rewriteConfig(api, os.Args[2])
	case "config": dmConfig(api, os.Args[2])
	case "ps": dPs(api, os.Args[2])
	case "startmachines": dmStart(api, os.Args[2:])
	case "start": clusterStart(api, os.Args[2:])
	case "destroy": clusterDestroy(api, os.Args[2:])
	case "url": url(api, os.Args[2:])
	default: fmt.Println("nope!")
	}
}
