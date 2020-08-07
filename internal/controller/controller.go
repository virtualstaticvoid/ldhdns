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
	"syscall"
	"time"
)

const (
	dnsContainerLabelPrefix = "dns.ldh"
	containerStopTimeout    = 30 * time.Second
)

type server struct {
	docker             *client.Client
	ctx                context.Context
	cancel             context.CancelFunc
	networkId          string
	domainSuffix       string
	subDomainLabel     string
	ownContainerId     string
	ownContainer       *types.ContainerJSON
	containerNetworkID string
	dnsContainer       *types.ContainerJSON
	linkObject         dbus.BusObject
}

func Run(networkId string, domainSuffix string, subDomainLabel string) error {
	log.Println("Starting...")
	s, err := newServer(networkId, domainSuffix, subDomainLabel)
	if err != nil {
		log.Println("Failed to start server: ", err)
		return err
	}
	defer s.close()

	log.Printf("Configured for %q domain and %q container label.", domainSuffix, subDomainLabel)

	log.Println("Starting DNS container...")
	err = s.findOrCreateAndRunDNSContainer()
	if err != nil {
		log.Println("Failed to start DNS container: ", err)
		return err
	}

	log.Println("Applying DNS change...")
	err = s.applyDNSConfiguration()
	if err != nil {
		log.Println("Failed to apply DNS change: ", err)
		return err
	}

	log.Println("Running event loop...")
	err = s.runEventLoop()
	if err != nil {
		log.Println("Failed to run event loop: ", err)
		return err
	}

	log.Println("Shutting down...")
	return nil
}

func newServer(networkId string, domainSuffix string, subDomainLabel string) (*server, error) {
	// connect to the docker API
	docker, err := client.NewEnvClient()
	if err != nil {
		log.Println("Failed to connect to Docker API: ", err)
		return nil, err
	}

	// context for background processing
	ctx, cancel := context.WithCancel(context.Background())

	svr := &server{
		docker:         docker,
		ctx:            ctx,
		cancel:         cancel,
		networkId:      networkId,
		domainSuffix:   domainSuffix,
		subDomainLabel: subDomainLabel,
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
	log.Println("Reverting DNS change...")
	err := s.revertDNSForLink()
	if err != nil {
		log.Printf("Failed to revert DNS: %s\n", err)
		// return err
	}

	log.Println("Stopping DNS container...")
	err = s.stopDNSContainer()
	if err != nil {
		log.Printf("Failed to stop container %s: %s\n", s.dnsContainer.ID, err)
		// return err
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

func (s *server) inspectOwnContainer() (*types.ContainerJSON, error) {
	var ownContainer types.ContainerJSON

	ownContainer, err := s.docker.ContainerInspect(s.ctx, s.ownContainerId)
	if err != nil {
		log.Printf("Failed to inspect container %s: %s\n", s.ownContainerId, err)
		return &ownContainer, err
	}

	// must be on host network
	if ownContainer.HostConfig.NetworkMode != "host" {
		log.Printf("Container %s isn't connected to the host network\n", s.ownContainerId)
		return &ownContainer, errors.New("container must be run in host network")
	}

	return &ownContainer, nil
}

func (s *server) findOrCreateNetwork() (string, error) {
	var containerNetworkID string

	// attempt to retrieve existing network
	containerNetwork, err := s.docker.NetworkInspect(s.ctx, s.networkId)
	if err != nil && client.IsErrNetworkNotFound(err) {
		// not found; create new bridge network
		log.Printf("Creating %s network...\n", s.networkId)
		networkCreateOptions := types.NetworkCreate{
			Driver: "bridge",
		}

		newNetwork, err := s.docker.NetworkCreate(s.ctx, s.networkId, networkCreateOptions)
		if err != nil {
			log.Printf("Failed to create network %s: %s\n", s.networkId, err)
			return containerNetworkID, err
		}
		containerNetworkID = newNetwork.ID

	} else if err != nil {
		log.Printf("Failed to inspect network %s: %s\n", s.networkId, err)
		return containerNetworkID, err
	} else {
		containerNetworkID = containerNetwork.ID
	}

	return containerNetworkID, nil
}

func (s *server) findOrCreateAndRunDNSContainer() error {
	var containerID string

	// formulate container name by convention
	// using the controllers container name pairs the two nicely
	// and allows more than one instance of the controller to run
	// for different domains and/or sub-domain labels if required
	// NB: no validation is done on the uniqueness of domain names
	// if multiple instances are running for the same domain
	// NOTE: container name has "/" prefix which needs to be removed
	containerName := fmt.Sprintf("%s_%s", s.ownContainer.Name[1:], "dns")

	// container already exists?
	dnsContainer, err := s.docker.ContainerInspect(s.ctx, containerName)
	if err != nil && client.IsErrNetworkNotFound(err) {
		// not found; create container using own image and bindings, using "dns" command
		log.Printf("Creating %s container...\n", containerName)

		labels := map[string]string{
			fmt.Sprintf("%s/%s", dnsContainerLabelPrefix, "controller-id"):   s.ownContainer.ID,
			fmt.Sprintf("%s/%s", dnsContainerLabelPrefix, "controller-name"): s.ownContainer.Name[1:],
			fmt.Sprintf("%s/%s", dnsContainerLabelPrefix, "network-id"):      s.networkId,
			fmt.Sprintf("%s/%s", dnsContainerLabelPrefix, "domain-suffix"):   s.domainSuffix,
			fmt.Sprintf("%s/%s", dnsContainerLabelPrefix, "subdomain-label"): s.subDomainLabel,
		}

		config := &container.Config{
			Image:  s.ownContainer.Config.Image,
			Cmd:    []string{"dns"},
			Env:    s.ownContainer.Config.Env,
			Labels: labels,
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
				s.networkId: {
					NetworkID: s.containerNetworkID,
				},
			},
		}

		newContainer, err := s.docker.ContainerCreate(s.ctx, config, hostConfig, networkingConfig, containerName)
		if err != nil {
			log.Printf("Failed to create container: %s\n", err)
			return err
		}
		containerID = newContainer.ID

	} else if err != nil {
		log.Printf("Failed to inspect container %s: %s\n", s.networkId, err)
		return err
	} else {

		containerID = dnsContainer.ID
	}

	err = s.docker.ContainerStart(s.ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		log.Printf("Failed to start container: %s\n", err)
		return err
	}

	// finally inspect the running container
	// since the network info is required
	dnsContainer, err = s.docker.ContainerInspect(s.ctx, containerID)
	if err != nil {
		log.Printf("Failed to inspect container %s: %s\n", containerID, err)
		return err
	}
	s.dnsContainer = &dnsContainer

	return nil
}

func (s *server) applyDNSConfiguration() error {
	// get the gateway IP address of the DNS container
	nw := s.dnsContainer.NetworkSettings.Networks[s.networkId]
	ipAddress := net.ParseIP(nw.IPAddress)
	gwIpAddress := net.ParseIP(nw.Gateway)
	if ipAddress == nil || gwIpAddress == nil {
		return errors.New("failed to parse IP addresses")
	}

	// get the index of the network interface on the host
	// this is why the process needs to run on the host network
	index, name, err := s.findNetworkInterfaceIndex(gwIpAddress)
	if err != nil {
		log.Printf("Failed to get network interface for %s: %s\n", gwIpAddress, err)
		return err
	}

	log.Printf("Applying configuration to %q network.\n", name)

	// register link DNS for this IP
	// keep the link object for the clean up later
	s.linkObject, err = s.setLinkDNSAndRoutingDomain(ipAddress, index)
	if err != nil {
		log.Printf("Failed to set DNS on link: %s\n", err)
		return err
	}

	return nil
}

func (s *server) findNetworkInterfaceIndex(ip net.IP) (int, string, error) {
	var name string
	netInterfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("Failed to get network interfaces: %s\n", err)
		return 0, name, err
	}
	for _, netInterface := range netInterfaces {
		addresses, err := netInterface.Addrs()
		if err == nil {
			for _, address := range addresses {
				ipAddr, ok := address.(*net.IPNet)
				if ok && !ipAddr.IP.IsLoopback() && !ipAddr.IP.IsMulticast() && ipAddr.IP.Equal(ip) {
					return netInterface.Index, netInterface.Name, nil
				}
			}
		}
	}
	return 0, name, errors.New("unable to determine index for network interface")
}

func (s *server) setLinkDNSAndRoutingDomain(address net.IP, index int) (dbus.BusObject, error) {
	// see LinkObject for interface details
	// https://www.freedesktop.org/wiki/Software/systemd/resolved/

	// SetDNS argument - a(iay)
	type Address struct {
		AddressFamily int32
		IpAddress     []uint8
	}

	// SetDomains argument - a(sb)
	type Domain struct {
		Name    string
		Routing bool
	}

	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %s", err)
	}

	var linkPath dbus.ObjectPath
	var callFlags dbus.Flags

	manager := conn.Object("org.freedesktop.resolve1", "/org/freedesktop/resolve1")
	err = manager.Call("org.freedesktop.resolve1.Manager.GetLink", callFlags, index).Store(&linkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get link: %s", err)
	}

	// merge with existing addresses & domains?

	var addresses []Address
	if address.To4() != nil {
		addresses = append(addresses, Address{
			AddressFamily: syscall.AF_INET,
			IpAddress:     address.To4(),
		})
	} else {
		addresses = append(addresses, Address{
			AddressFamily: syscall.AF_INET6,
			IpAddress:     address.To16(),
		})
	}

	var domains []Domain
	domains = append(domains, Domain{
		Name:    s.domainSuffix,
		Routing: true,
	})

	// update link with new DNS server address
	link := conn.Object("org.freedesktop.resolve1", linkPath)
	result1 := link.Call("org.freedesktop.resolve1.Link.SetDNS", callFlags, addresses)
	if result1.Err != nil {
		return nil, fmt.Errorf("failed to set link DNS: %s", result1.Err)
	}

	// update link with routing domain name
	result2 := link.Call("org.freedesktop.resolve1.Link.SetDomains", callFlags, domains)
	if result2.Err != nil {
		return nil, fmt.Errorf("failed to set link Domain: %s", result2.Err)
	}

	return link, nil
}

func (s *server) runEventLoop() error {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	// wait to be signalled
	sig := <-signals
	log.Printf("Received %s signal\n", sig.String())
	return nil
}

func (s *server) revertDNSForLink() error {
	// see LinkObject for interface details
	// https://www.freedesktop.org/wiki/Software/systemd/resolved/

	if s.linkObject == nil {
		return nil
	}

	var callFlags dbus.Flags
	result := s.linkObject.Call("org.freedesktop.resolve1.Link.Revert", callFlags)
	if result.Err != nil {
		return fmt.Errorf("failed to revert link DNS: %s", result.Err)
	}

	return nil
}

func (s *server) stopDNSContainer() error {
	if len(s.dnsContainer.ID) == 0 {
		return nil
	}

	var timeout = containerStopTimeout
	err := s.docker.ContainerStop(s.ctx, s.dnsContainer.ID, &timeout)
	if err != nil {
		return fmt.Errorf("failed to stop DNS container: %s", err)
	}

	return nil
}
