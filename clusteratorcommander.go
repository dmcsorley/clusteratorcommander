package main

import (
	//"encoding/json"
	"fmt"
	"github.com/docker/machine/commands/mcndirs"
	"github.com/docker/machine/libmachine"
	//"github.com/docker/machine/libmachine/check"
	"log"
	"os"
)

func getHost(api *libmachine.Client, hostname string) *host.Host {
	host, err := api.Load(hostname)
	if (err != nil) {
		log.Fatal(err)
	}
}

func rewriteConfig(api *libmachine.Client, hostname string) {
	host = getHost(api, hostname)
	ip, _ := host.Driver.GetIP()

	swarmopts := host.HostOptions.SwarmOptions
	swarmopts.IsSwarm = true
	swarmopts.Master = true
	swarmopts.Discovery = fmt.Sprintf("consul://%s:8500/barney", ip)

	api.Save(host)
}

func printIP(api *libmachine.Client, hostname string) {
	host = getHost(api, hostname)
	ip, _ := host.Driver.GetIP()
	fmt.Println(ip)
}

func printConfig(api *libmachine.Client, hostname string) {
	host = getHost(api, hostname)
	prettyJSON, err := json.MarshalIndent(host, "", "    ")
	if err != nil {
		log.Fatal(err)
   	}

   	fmt.Println(string(prettyJSON))
}

func main() {
	command := os.Args[1]

    api := libmachine.NewClient(mcndirs.GetBaseDir(), mcndirs.GetMachineCertDir())
	defer api.Close()

	machine := os.Args[1]

    st, _ := host.Driver.GetURL()
	//st, _, _ := check.DefaultConnChecker.Check(host, false)
	fmt.Println(st)
}
