package controller

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/godbus/dbus"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
)

const (
	networkID = "ldhdns"
)

type server struct {
	lock               sync.RWMutex
	docker             *client.Client
	ctx                context.Context
	cancel             context.CancelFunc
	domain             string
	ownContainerId     string
	ownContainer       types.ContainerJSON
	containerNetworkID string
	dnsContainerId     string
	dnsContainer       types.ContainerJSON
	linkObject         dbus.BusObject
}

func Run(domain string) error {
	log.Printf("Starting...")
	server, err := newServer(domain)
	if err != nil {
		log.Println("Failed to start server: ", err)
		return err
	}
	defer server.close()

	log.Println("Starting DNS container...")
	err = server.runDNSContainer()
	if err != nil {
		log.Println("Failed to start DNS container: ", err)
		return err
	}

	log.Println("Applying DNS change...")
	err = server.applyDNSConfiguration()
	if err != nil {
		log.Println("Failed to apply DNS change: ", err)
		return err
	}

	log.Println("Running event loop...")
	err = server.runEventLoop()
	if err != nil {
		log.Println("Failed to run event loop: ", err)
		return err
	}

	log.Println("Shutting down...")
	return nil
}

func newServer(domain string) (*server, error) {
	// connect to the docker API
	docker, err := client.NewEnvClient()
	if err != nil {
		log.Println("Failed to connect to Docker API: ", err)
		return nil, err
	}

	// context for background processing
	ctx, cancel := context.WithCancel(context.Background())

	svr := &server{
		docker: docker,
		ctx:    ctx,
		cancel: cancel,
		domain: domain,
	}

	svr.ownContainerId, err = svr.findOwnContainerId()
	if err != nil {
		log.Println("Failed to determine own container ID: ", err)
		return nil, err
	}

	svr.ownContainer, err = svr.inspectOwnContainer()
	if err != nil {
		log.Println("Failed to inspect own container ID: ", err)
		return nil, err
	}

	svr.containerNetworkID, err = svr.findOrCreateNetwork()
	if err != nil {
		log.Println("Failed to find or create container network: ", err)
		return nil, err
	}

	return svr, nil
}

func (s *server) close() error {
	err := s.revertDNSForLink()
	if err != nil {
		log.Printf("Failed to revert DNS: %s\n", err)
		return err
	}

	err = s.docker.ContainerStop(s.ctx, s.dnsContainer.ID, nil)
	if err != nil {
		log.Printf("Failed to stop container %s: %s\n", s.dnsContainer.ID, err)
		return err
	}

	return s.docker.Close()
}

func (s *server) findOwnContainerId() (string, error) {
	var ownContainerId string

	// HACK: get own container information
	// undocumented feature...
	// this might break in future
	file, err := os.Open("/proc/1/cpuset")
	if err != nil {
		log.Println("Failed to open /proc/1/cpuset: ", err)
		return ownContainerId, err
	}
	defer file.Close()

	// only need the first line, without \n
	var firstLine string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		firstLine = scanner.Text()
		break
	}

	// extract the last part of path
	ownContainerId = filepath.Base(firstLine)
	if len(ownContainerId) < 2 {
		return ownContainerId, errors.New("not executing within a container")
	}

	return ownContainerId, nil
}

func (s *server) inspectOwnContainer() (types.ContainerJSON, error) {
	var ownContainer types.ContainerJSON

	ownContainer, err := s.docker.ContainerInspect(s.ctx, s.ownContainerId)
	if err != nil {
		log.Printf("Failed to inspect container %s: %s\n", s.ownContainerId, err)
		return ownContainer, err
	}

	// must be on host network
	if ownContainer.HostConfig.NetworkMode != "host" {
		log.Printf("Container %s isn't connected to the host network\n", s.ownContainerId)
		return ownContainer, errors.New("container must be run in host network")
	}

	return ownContainer, nil
}

func (s *server) findOrCreateNetwork() (string, error) {
	var containerNetworkID string

	// attempt to retrieve existing network
	containerNetwork, err := s.docker.NetworkInspect(s.ctx, networkID)
	if err != nil && client.IsErrNetworkNotFound(err) {
		// not found; create new bridge network
		log.Printf("Creating %s network\n", networkID)
		networkCreateOptions := types.NetworkCreate{
			Driver: "bridge",
		}

		newNetwork, err := s.docker.NetworkCreate(s.ctx, networkID, networkCreateOptions)
		if err != nil {
			log.Printf("Failed to create network %s: %s\n", networkID, err)
			return containerNetworkID, err
		}
		containerNetworkID = newNetwork.ID

	} else if err != nil {
		log.Printf("Failed to inspect network %s: %s\n", networkID, err)
		return containerNetworkID, err
	} else {
		containerNetworkID = containerNetwork.ID
	}

	return containerNetworkID, nil
}

func (s *server) runDNSContainer() error {
	// create image using own container image and bindings, using "dns" command
	config := &container.Config{
		Image: s.ownContainer.Config.Image,
		Cmd:   []string{"dns"},
	}

	// Note: needs CAP_NET_ADMIN capabilities
	hostConfig := &container.HostConfig{
		AutoRemove: true,
		Binds:      s.ownContainer.HostConfig.Binds,
		CapAdd:     []string{"CAP_NET_ADMIN"},
	}

	// supply the bridge network we created
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkID: {
				NetworkID: s.containerNetworkID,
			},
		},
	}

	log.Println("Starting DNS server container")
	dnsContainer, err := s.docker.ContainerCreate(s.ctx, config, hostConfig, networkingConfig, "")
	if err != nil {
		log.Printf("Failed to create container: %s\n", err)
		return err
	}

	err = s.docker.ContainerStart(s.ctx, dnsContainer.ID, types.ContainerStartOptions{})
	if err != nil {
		log.Printf("Failed to start container: %s\n", err)
		return err
	}

	s.dnsContainer, err = s.docker.ContainerInspect(s.ctx, dnsContainer.ID)
	if err != nil {
		log.Printf("Failed to inspect container %s: %s\n", dnsContainer.ID, err)
		return err
	}

	return nil
}

func (s *server) applyDNSConfiguration() error {
	// get the gateway IP address of the DNS container
	nw := s.dnsContainer.NetworkSettings.Networks[networkID]
	ipAddress := net.ParseIP(nw.IPAddress)
	gwIpAddress := net.ParseIP(nw.Gateway)
	if ipAddress == nil || gwIpAddress == nil {
		return errors.New("failed to parse IP addresses")
	}

	// get the index of the network interface on the host
	// this is why the process needs to run on the host network
	index, err := s.findNetworkInterfaceIndexFor(gwIpAddress)
	if err != nil {
		log.Printf("Failed to get network interface for %s: %s\n", gwIpAddress, err)
		return err
	}

	// register link DNS for this IP
	// keep the link object for the clean up later
	s.linkObject, err = s.setDNSForLink(ipAddress, index)
	if err != nil {
		log.Printf("Failed to set DNS on link: %s\n", err)
		return err
	}

	return nil
}

func (s *server) findNetworkInterfaceIndexFor(ip net.IP) (int, error) {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("Failed to get network interfaces: %s\n", err)
		return 0, err
	}
	for _, netInterface := range netInterfaces {
		addresses, err := netInterface.Addrs()
		if err == nil {
			for _, address := range addresses {
				ipAddr, ok := address.(*net.IPNet)
				if ok && !ipAddr.IP.IsLoopback() && !ipAddr.IP.IsMulticast() && ipAddr.IP.Equal(ip) {
					return netInterface.Index, nil
				}
			}
		}
	}
	return 0, errors.New("unable to determine index for network interface")
}

func (s *server) setDNSForLink(address net.IP, index int) (dbus.BusObject, error) {

	type Address struct {
		AddressFamily int32
		IpAddress     []uint8
	}

	type Domain struct {
		Name    string
		Routing bool
	}

	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %v", err)
	}

	var linkPath dbus.ObjectPath
	var callFlags dbus.Flags

	manager := conn.Object("org.freedesktop.resolve1", "/org/freedesktop/resolve1")
	err = manager.Call("org.freedesktop.resolve1.Manager.GetLink", callFlags, index).Store(&linkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get link: %v", err)
	}

	// merge with existing addresses & domains?

	var addresses []Address
	if address.To4() != nil {
		addresses = append(addresses, Address{
			AddressFamily: 2, // AF_INET
			IpAddress:     address.To4(),
		})
	} else {
		addresses = append(addresses, Address{
			AddressFamily: 10, // AF_INET6
			IpAddress:     address.To16(),
		})
	}

	var domains []Domain
	domains = append(domains, Domain{
		Name:    s.domain,
		Routing: true,
	})

	// update link with new DNS server address
	link := conn.Object("org.freedesktop.resolve1", linkPath)
	result1 := link.Call("org.freedesktop.resolve1.Link.SetDNS", callFlags, addresses)
	if result1.Err != nil {
		return nil, fmt.Errorf("failed to set link DNS: %v", result1.Err)
	}

	// update link with routing domain name
	result2 := link.Call("org.freedesktop.resolve1.Link.SetDomains", callFlags, domains)
	if result2.Err != nil {
		return nil, fmt.Errorf("failed to set link Domain: %v", result2.Err)
	}

	return link, nil
}

func (s *server) revertDNSForLink() error {
	var callFlags dbus.Flags

	result := s.linkObject.Call("org.freedesktop.resolve1.Link.Revert", callFlags)
	if result.Err != nil {
		return fmt.Errorf("failed to revert link DNS: %v", result.Err)
	}

	return nil
}

func (s *server) runEventLoop() error {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	// wait to be signalled
	<-signals
	return nil
}
