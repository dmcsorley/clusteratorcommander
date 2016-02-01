package main

import (
	"encoding/json"
	"fmt"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/docker/machine/commands/mcndirs"
	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/auth"
	"github.com/docker/machine/libmachine/check"
	"github.com/docker/machine/libmachine/host"
	"log"
	"net/http"
	"os"
)

func loadHost(api *libmachine.Client, hostname string) *host.Host {
	host, err := api.Load(hostname)
	if (err != nil) {
		log.Fatal(err)
	}
	return host
}

func getHostOptions(host *host.Host, swarm bool) (string, *auth.Options) {
	dockerHost, authOptions, err := check.DefaultConnChecker.Check(host, true)
	if err != nil {
		log.Fatal("Error running connection boilerplate: %s", err)
	}

	return dockerHost, authOptions
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

func forAllHosts(api *libmachine.Client, hostnames []string, applicable hostable) {
	for _, hostname := range hostnames {
		host := loadHost(api, hostname)
		applicable(host)
	}
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

func getClient(dockerHost string, authOptions *auth.Options) *client.Client {
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

func dPs(api *libmachine.Client, hostname string) {
	host := loadHost(api, hostname)
	dockerHost, authOptions := getHostOptions(host, true)
	cli := getClient(dockerHost, authOptions)

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
	default: fmt.Println("nope!")
	}

	//machine := os.Args[1]

	//st, _ := host.Driver.GetURL()
	//st, _, _ := check.DefaultConnChecker.Check(host, false)
	//fmt.Println(st)
}
