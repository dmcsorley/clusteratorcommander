package libclusterator

import (
	"fmt"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/auth"
	"github.com/docker/machine/libmachine/check"
	"github.com/docker/machine/libmachine/host"
	"golang.org/x/net/context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

func LoadHost(api *libmachine.Client, hostname string) *host.Host {
	host, err := api.Load(hostname)
	if (err != nil) {
		log.Fatal(err)
	}
	return host
}

func GetHostOptions(host *host.Host, swarm bool) (string, *auth.Options) {
	dockerHost, authOptions, err := check.DefaultConnChecker.Check(host, swarm)
	if err != nil {
		log.Fatal("Error running connection boilerplate: %s", err)
	}

	return dockerHost, authOptions
}

func CreateClient(dockerHost string, authOptions *auth.Options) *client.Client {
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

func RewriteConfig(api *libmachine.Client, hostname string) {
	host := LoadHost(api, hostname)
	ip, _ := host.Driver.GetIP()

	swarmopts := host.HostOptions.SwarmOptions
	swarmopts.IsSwarm = true
	swarmopts.Master = true
	swarmopts.Discovery = fmt.Sprintf("consul://%s:8500/barney", ip)

	api.Save(host)
}

type Hostable func(*host.Host)
type indexedhostable func(int, *host.Host)

func forAllHostsIndexed(api *libmachine.Client, hostnames []string, applicable indexedhostable) {
	for index, hostname := range hostnames {
		host := LoadHost(api, hostname)
		applicable(index, host)
	}
}

func ForAllHosts(api *libmachine.Client, hostnames []string, applicable Hostable) {
	forAllHostsIndexed(api, hostnames, func(index int, host *host.Host) {
		applicable(host)
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

func RunImage(
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

func ForceRemoveContainer(cli *client.Client, names []string) {
	for _, name := range names {
		err := cli.ContainerRemove(types.ContainerRemoveOptions{
			ContainerID: name,
			RemoveVolumes: true,
			Force: true,
		})
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("Removed", name)
		}
	}
}

