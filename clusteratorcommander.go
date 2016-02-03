package main

import (
	"encoding/json"
	"fmt"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/docker/machine/commands/mcndirs"
	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/auth"
	"github.com/docker/machine/libmachine/check"
	"github.com/docker/machine/libmachine/host"
	"golang.org/x/net/context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

const (
	CONSUL_CONTAINER_NAME = "clusterator_consul"
	CONSUL_AMD64_IMAGE = "progrium/consul"
)

func loadHost(api *libmachine.Client, hostname string) *host.Host {
	host, err := api.Load(hostname)
	if (err != nil) {
		log.Fatal(err)
	}
	return host
}

func getHostOptions(host *host.Host, swarm bool) (string, *auth.Options) {
	dockerHost, authOptions, err := check.DefaultConnChecker.Check(host, swarm)
	if err != nil {
		log.Fatal("Error running connection boilerplate: %s", err)
	}

	return dockerHost, authOptions
}

func createClient(dockerHost string, authOptions *auth.Options) *client.Client {
	// based on docker/engine-api/client.NewEnvClient
	options := tlsconfig.Options{
		CAFile:             authOptions.CaCertPath,
		CertFile:           authOptions.ClientCertPath,
		KeyFile:            authOptions.ClientKeyPath,
		InsecureSkipVerify: false,
	}

	tlsc, err := tlsconfig.Client(options)
	if err != nil {
		log.Fatal(err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsc,
		},
	}

	cli, err := client.NewClient(dockerHost, "v1.21", httpClient, nil)
	if err != nil {
		log.Fatal(err)
	}

	return cli
}

func rewriteConfig(api *libmachine.Client, hostname string) {
	host := loadHost(api, hostname)
	ip, _ := host.Driver.GetIP()

	swarmopts := host.HostOptions.SwarmOptions
	swarmopts.IsSwarm = true
	swarmopts.Master = true
	swarmopts.Discovery = fmt.Sprintf("consul://%s:8500/barney", ip)

	api.Save(host)
}

type hostable func(*host.Host)
type indexedhostable func(int, *host.Host)

func forAllHostsIndexed(api *libmachine.Client, hostnames []string, applicable indexedhostable) {
	for index, hostname := range hostnames {
		host := loadHost(api, hostname)
		applicable(index, host)
	}
}

func forAllHosts(api *libmachine.Client, hostnames []string, applicable hostable) {
	forAllHostsIndexed(api, hostnames, func(index int, host *host.Host) {
		applicable(host)
	})
}

func dmIP(api *libmachine.Client, hostnames []string) {
	forAllHosts(api, hostnames, func(host *host.Host) {
		ip, _ := host.Driver.GetIP()
		fmt.Println(host.Name, ip)
	})
}

func printJson(api *libmachine.Client, hostname string) {
	host := loadHost(api, hostname)
	prettyJSON, err := json.MarshalIndent(host, "", "    ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(prettyJSON))
}

func dmConfig(api *libmachine.Client, hostname string) {
	host := loadHost(api, hostname)
	dockerHost, authOptions := getHostOptions(host, true)

	fmt.Printf("--tlsverify\n--tlscacert=%q\n--tlscert=%q\n--tlskey=%q\n-H=%s\n",
		authOptions.CaCertPath, authOptions.ClientCertPath, authOptions.ClientKeyPath, dockerHost)
}

func dPs(api *libmachine.Client, hostname string) {
	host := loadHost(api, hostname)
	dockerHost, authOptions := getHostOptions(host, true)
	cli := createClient(dockerHost, authOptions)

	containers, err := cli.ContainerList(types.ContainerListOptions{All: true})
	if err != nil {
		panic(err)
	}

	for _, c := range containers {
		fmt.Println(c.ID)
	}
}

func dmStart(api *libmachine.Client, hostnames []string) {
	forAllHosts(api, hostnames, func(host *host.Host) {
		host.Start()
	})
}

func pull(cli *client.Client, image, tag string) error {
	pullOptions := types.ImagePullOptions{
		ImageID: image,
		Tag: tag,
	}

	if readCloser, err := cli.ImagePull(context.Background(), pullOptions, nil); err != nil {
		return err
	} else {
		fmt.Printf("Downloading %s:%s\n", image, tag)
		io.Copy(ioutil.Discard, readCloser)
		readCloser.Close()
		fmt.Println("Done")
		return nil
	}
}

func runImage(
	cli *client.Client,
	containerConfig *container.Config,
	hostConfig *container.HostConfig,
	containerName string,
) error {
	createResponse, err := cli.ContainerCreate(containerConfig, hostConfig, nil, containerName)

	if err != nil {
		if client.IsErrImageNotFound(err) {
			pull(cli, containerConfig.Image, "latest")
			createResponse, err = cli.ContainerCreate(containerConfig, hostConfig, nil, containerName)
		} else {
			return err
		}
	}

	fmt.Println("Created", containerName, createResponse.ID)

	err = cli.ContainerStart(createResponse.ID)
	if err != nil {
		return err
	}

	return nil
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

	return runImage(cli, containerConfig, hostConfig, CONSUL_CONTAINER_NAME)
}

func startFirstMember(api *libmachine.Client, hostname string, quorum int) (string, error) {
	host := loadHost(api, hostname)
	ip, _ := host.Driver.GetIP()
	dockerHost, authOptions := getHostOptions(host, false)
	cli := createClient(dockerHost, authOptions)
	err := startConsul(cli, strslice.New("-server", "-bind", ip, "-bootstrap-expect", strconv.Itoa(quorum)))
	if err != nil {
		return "", err
	}

	return ip, nil
}

func clStart(api *libmachine.Client, hostnames []string) {
	quorum := len(hostnames)/2 + 1
	ip, err := startFirstMember(api, hostnames[0], quorum)
	if err != nil {
		fmt.Println(err);
		return
	}
	//fmt.Println(quorum, CONSUL_CONTAINER_NAME)
	fmt.Println(ip)

	//forAllHostsIndexed(api, hostnames, func(index int, host *host.Host) {
		//ip, _ := host.Driver.GetIP()
		//fmt.Println(index, host.Name, ip)
		//dockerHost, authOptions := getHostOptions(host, false)
		//cli := createClient(dockerHost, authOptions)

		//containerConfig := &container.Config{
			//Image: CONSUL_AMD64_IMAGE,
			//Cmd: []string{"-server", "-bind", ip, "-bootstrap-expect", quorum}
		//}
//CONSUL_QUORUM="-bootstrap-expect $(($MACHINE_COUNT / 2 + 1))"
//CONSUL_JOINABLE="-join $(docker-machine ip $FIRST):8301"
// progrium/consul
// net: host
// log_opt:
//max-size: "10m"
//max-file: "5"
// command: -server -bind ${MACHINE_IP} ${CONSUL_QUORUM_OR_JOIN}
//FIRST=${MACHINE_NAMES[0]}
	//})
}

func clDestroy(api *libmachine.Client, hostnames []string) {
	forAllHosts(api, hostnames, func(host *host.Host) {
		dockerHost, authOptions := getHostOptions(host, false)
		cli := createClient(dockerHost, authOptions)
		err := cli.ContainerRemove(types.ContainerRemoveOptions{
			ContainerID: CONSUL_CONTAINER_NAME,
			RemoveVolumes: true,
			Force: true,
		})
		if err != nil {
			fmt.Println(err)
		}
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
	case "start": dmStart(api, os.Args[2:])
	case "startcluster": clStart(api, os.Args[2:])
	case "destroy": clDestroy(api, os.Args[2:])
	default: fmt.Println("nope!")
	}

	//machine := os.Args[1]

	//st, _ := host.Driver.GetURL()
	//st, _, _ := check.DefaultConnChecker.Check(host, false)
	//fmt.Println(st)
}
