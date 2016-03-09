package libclusterator

import (
	"fmt"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/auth"
	"github.com/docker/machine/libmachine/check"
	"github.com/docker/machine/libmachine/host"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type DockerConnection interface {
	RunImage(containerConfig *container.Config, hostConfig *container.HostConfig, containerName string) error
	ForceRemoveContainers(containers []string)
	GetDockerURL() DockerURL
	GetDiscoveryURL(swarmname string) string
	SaveSwarmConfig(swarmname string) error
}

type DockerMachineConnection struct {
	api *libmachine.Client
	host *host.Host
	url DockerURL
	authOptions *auth.Options
	client *client.Client
}

func NewConnection(api *libmachine.Client, hostname string) *DockerMachineConnection {
	host, err := api.Load(hostname)
	if (err != nil) {
		log.Fatal(err)
	}
	dockerHost, authOptions, err := check.DefaultConnChecker.Check(host, false)
	if err != nil {
		log.Fatal("Error running connection boilerplate:", err)
	}

	client := createClient(dockerHost, authOptions)

	return &DockerMachineConnection{
		api: api,
		host: host,
		url: &StringDockerURL{url:dockerHost},
		authOptions: authOptions,
		client: client,
	}
}

type DockerURL interface {
	GetUrl() string
	GetHostPort() string
	GetHost() string
}

type StringDockerURL struct {
	url string
}

func (url *StringDockerURL) GetUrl() string {
	return url.url
}

func (url *StringDockerURL) GetHostPort() string {
	parts := strings.SplitN(url.url, "://", 2)
	return parts[1]
}

func (url *StringDockerURL) GetHost() string {
	parts := strings.SplitN(url.GetHostPort(), ":", 2)
	return parts[0]
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

type Machinable func(DockerConnection)

func ForAllMachines(api *libmachine.Client, hostnames []string, applicable Machinable) {
	for _, hostname := range hostnames {
		connection := NewConnection(api, hostname)
		applicable(connection)
	}
}

func pull(cli *client.Client, image, tag string) error {
	pullOptions := types.ImagePullOptions{
		ImageID: image,
		Tag: tag,
	}

	if response, err := cli.ImagePull(context.Background(), pullOptions, nil); err != nil {
		return err
	} else {
		defer response.Close()
		jsonmessage.DisplayJSONMessagesStream(response, os.Stdout, os.Stdout.Fd(), true, nil)
		return nil
	}
}

func (conn *DockerMachineConnection) RunImage(
	containerConfig *container.Config,
	hostConfig *container.HostConfig,
	containerName string,
) error {
	createResponse, err := conn.client.ContainerCreate(context.Background(), containerConfig, hostConfig, nil, containerName)

	if err != nil {
		if client.IsErrImageNotFound(err) {
			pull(conn.client, containerConfig.Image, "latest")
			createResponse, err = conn.client.ContainerCreate(context.Background(), containerConfig, hostConfig, nil, containerName)
		} else {
			return err
		}
	}

	fmt.Printf("Created %s/%s %s\n", conn.host.Name, containerName, createResponse.ID)

	err = conn.client.ContainerStart(context.Background(), createResponse.ID)
	if err != nil {
		return err
	}

	time.Sleep(500*time.Millisecond)

	return nil
}

func (conn *DockerMachineConnection) ForceRemoveContainers(names []string) {
	for _, name := range names {
		err := conn.client.ContainerRemove(context.Background(), types.ContainerRemoveOptions{
			ContainerID: name,
			RemoveVolumes: true,
			Force: true,
		})
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Printf("Removed %s/%s\n", conn.host.Name, name)
		}
	}
}

func (conn *DockerMachineConnection) GetDockerURL() DockerURL {
	return conn.url
}

func (conn *DockerMachineConnection) GetDiscoveryURL(clustername string) string {
	return "consul://" + conn.url.GetHost() + ":8500/" + clustername
}

func (conn *DockerMachineConnection) SaveSwarmConfig(clustername string) error {
	host, err := conn.api.Load(conn.host.Name)
	if err != nil {
		return err
	}

	swarmopts := host.HostOptions.SwarmOptions
	swarmopts.IsSwarm = true
	swarmopts.Master = true
	swarmopts.Discovery = conn.GetDiscoveryURL(clustername)
	conn.host = host

	return conn.api.Save(host)
}

