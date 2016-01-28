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

func getHost(api *libmachine.Client, hostname string) *host.Host {
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
	host := getHost(api, hostname)
	ip, _ := host.Driver.GetIP()

	swarmopts := host.HostOptions.SwarmOptions
	swarmopts.IsSwarm = true
	swarmopts.Master = true
	swarmopts.Discovery = fmt.Sprintf("consul://%s:8500/barney", ip)

	api.Save(host)
}

func printIP(api *libmachine.Client, hostname string) {
	host := getHost(api, hostname)
	ip, _ := host.Driver.GetIP()
	fmt.Println(ip)
}

func printJson(api *libmachine.Client, hostname string) {
	host := getHost(api, hostname)
	prettyJSON, err := json.MarshalIndent(host, "", "    ")
	if err != nil {
		log.Fatal(err)
   	}

   	fmt.Println(string(prettyJSON))
}

func printConfig(api *libmachine.Client, hostname string) {
	host := getHost(api, hostname)
	dockerHost, authOptions := getHostOptions(host, true)

	fmt.Printf("--tlsverify\n--tlscacert=%q\n--tlscert=%q\n--tlskey=%q\n-H=%s\n",
		authOptions.CaCertPath, authOptions.ClientCertPath, authOptions.ClientKeyPath, dockerHost)
}

func getClient(dockerHost string, authOptions *auth.Options) *client.Client {
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

	transport := &http.Transport{
		TLSClientConfig: tlsc,
	}

	cli, err := client.NewClient(dockerHost, "v1.21", transport, nil)
	if err != nil {
		log.Fatal(err)
	}

	return cli
}

func ps(api *libmachine.Client, hostname string) {
	host := getHost(api, hostname)
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

func start(api *libmachine.Client, hostnames []string) {
	for _, hostname := range hostnames {
		host := getHost(api, hostname)
		host.Start()
	}
}

func main() {
	api := libmachine.NewClient(mcndirs.GetBaseDir(), mcndirs.GetMachineCertDir())
	defer api.Close()

	command := os.Args[1]

	switch command {
	case "ip": printIP(api, os.Args[2])
	case "json": printJson(api, os.Args[2])
	case "rewrite": rewriteConfig(api, os.Args[2])
	case "config": printConfig(api, os.Args[2])
	case "ps": ps(api, os.Args[2])
	case "start": start(api, os.Args[2:])
	default: fmt.Println("nope!")
	}

	//machine := os.Args[1]

	//st, _ := host.Driver.GetURL()
	//st, _, _ := check.DefaultConnChecker.Check(host, false)
	//fmt.Println(st)
}
