package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/godbus/dbus/v5"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	dnsContainerLabelPrefix = "dns.ldh"
	containerStopTimeout    = 30 * time.Second
	dbusChannelBufferSize   = 10

	dbusResolveInterface        = "org.freedesktop.resolve1"
	dbusResolveManagerInterface = "org.freedesktop.resolve1.Manager"
	dbusResolvePath             = "/org/freedesktop/resolve1"
	dbusResolveGetLinkMethod    = "org.freedesktop.resolve1.Manager.GetLink"
	dbusResolveSetDNSMethod     = "org.freedesktop.resolve1.Link.SetDNS"
	dbusResolveSetDomainsMethod = "org.freedesktop.resolve1.Link.SetDomains"

	dbusBecomeMonitorMethod     = "org.freedesktop.DBus.Monitoring.BecomeMonitor"
	dbusLoginManagerInterface   = "org.freedesktop.login1.Manager"
	dbusPrepareForSleepSignal   = "PrepareForSleep"
	dbusPropertiesInterface     = "org.freedesktop.DBus.Properties"
	dbusPropertiesChangedSignal = "PropertiesChanged"
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
	systemBus          *dbus.Conn
	linkIndex          int
	linkObject         dbus.BusObject
}

func Run(networkId string, domainSuffix string, subDomainLabel string, containerName string) error {
	log.Println("Starting...")
	s, err := newServer(networkId, domainSuffix, subDomainLabel, containerName)
	if err != nil {
		log.Println("Failed to start server: ", err)
		return err
	}

	log.Printf("Configured for %q domain and %q container label.\n", domainSuffix, subDomainLabel)

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
	err = s.close()
	if err != nil {
		log.Println("Failed shutdown: ", err)
		return err
	}

	log.Println("Bye...")
	return nil
}

func newServer(networkId string, domainSuffix string, subDomainLabel string, containerName string) (*server, error) {
	// connect to the docker API
	docker, err := client.NewClientWithOpts(client.FromEnv)
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

	svr.ownContainerId, err = svr.findOwnContainerId(containerName)
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

	// open private connection to session bus
	// since eves-dropping on dbus.SystemBus
	svr.systemBus, err = svr.connectSessionBus()
	if err != nil {
		log.Println("Failed to connect to system bus: ", err)
		return nil, fmt.Errorf("failed to connect to system bus: %s", err)
	}

	return svr, nil
}

func (s *server) close() error {

	log.Println("Reverting DNS change...")
	err := s.revertDNSForLink()
	if err != nil {
		log.Println("Failed to revert DNS: ", err)
		// return err
	}

	log.Println("Stopping DNS container...")
	err = s.stopDNSContainer()
	if err != nil {
		log.Printf("Failed to stop container %s: %s\n", s.dnsContainer.ID, err)
		// return err
	}

	// close connection to system bus
	log.Println("Closing SystemBus connection...")
	if s.systemBus != nil {
		err = s.systemBus.Close()
		if err != nil {
			log.Println("Failed to close SystemDbus connection: ", err)
			// return err
		}
	}

	// close connection to docker
	log.Println("Closing docker connection...")
	err = s.docker.Close()
	if err != nil {
		log.Println("Failed to close SystemDbus connection: ", err)
		// return err
	}

	return nil
}

func (s *server) findOwnContainerId(containerName string) (string, error) {
	var ownContainerId string

	log.Printf("Attempting to obtain container ID for %q container...\n", containerName)
	listOptions := types.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.KeyValuePair{
				Key:   "name",
				Value: fmt.Sprintf("^%s$", containerName),
			},
		),
	}

	matches, err := s.docker.ContainerList(s.ctx, listOptions)
	if err != nil {
		log.Println("Failed to retrieve container list: ", err)
		return ownContainerId, err
	}

	// FIXME: handle multiple matches; taking the 1st match for now 🤞
	if len(matches) > 0 {
		ownContainerId = matches[0].ID
	} else {
		log.Println("Fatal: Unable to obtain container ID!")
		return ownContainerId, errors.New("unable to obtain container id")
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
	options := types.NetworkInspectOptions{}

	// attempt to retrieve existing network
	containerNetwork, err := s.docker.NetworkInspect(s.ctx, s.networkId, options)
	if err != nil && client.IsErrNotFound(err) {
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

func (s *server) connectSessionBus() (*dbus.Conn, error) {
	conn, err := dbus.SystemBusPrivate()
	if err != nil {
		log.Println("Failed to connect to system bus: ", err)
		return nil, fmt.Errorf("failed to connect to system bus: %s", err)
	}

	err = conn.Auth(nil)
	if err != nil {
		log.Println("Failed to authenticate to system bus: ", err)
		_ = conn.Close()
		return nil, err
	}

	err = conn.Hello()
	if err != nil {
		log.Println("Failed hello to system bus: ", err)
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
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
	containerName := fmt.Sprintf("%s_%s", s.ownContainer.Name[1:], s.ownContainerId[:12])

	// container already exists?
	dnsContainer, err := s.docker.ContainerInspect(s.ctx, containerName)
	if err != nil && client.IsErrNotFound(err) {
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
			Image:      s.ownContainer.Config.Image,
			Entrypoint: []string{"/init"}, // s6-overlay entrypoint
			Env:        s.ownContainer.Config.Env,
			Labels:     labels,
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

		var platform *specs.Platform

		newContainer, err := s.docker.ContainerCreate(s.ctx, config, hostConfig, networkingConfig, platform, containerName)
		if err != nil {
			log.Println("Failed to create container: ", err)
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
		log.Println("Failed to start container: ", err)
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

	// sanity check
	if !dnsContainer.State.Running {
		log.Printf("Container unexpectantly exited [%d]\n", dnsContainer.State.ExitCode)
		return errors.New(fmt.Sprintf("container unexpectantly exited [%d]\n", dnsContainer.State.ExitCode))
	}

	return nil
}

func (s *server) applyDNSConfiguration() error {
	// get the gateway IP address of the DNS container
	nw := s.dnsContainer.NetworkSettings.Networks[s.networkId]
	ipAddress := net.ParseIP(nw.IPAddress)
	gwIpAddress := net.ParseIP(nw.Gateway)
	if ipAddress == nil || gwIpAddress == nil {
		log.Println("Failed to parse IP addresses")
		return errors.New("failed to parse IP addresses")
	}

	// get the index of the network interface on the host
	// this is why the process needs to run on the host network
	linkIndex, name, err := s.findNetworkInterfaceIndex(gwIpAddress)
	if err != nil {
		log.Printf("Failed to get network interface for %s: %s\n", gwIpAddress, err)
		return err
	}
	s.linkIndex = linkIndex

	log.Printf("Applying configuration to %q network.\n", name)

	// register link DNS for this IP
	// keep the link object for the clean up later
	s.linkObject, err = s.setLinkDNSAndRoutingDomain(ipAddress)
	if err != nil {
		log.Println("Failed to set DNS on link: ", err)
		return err
	}

	return nil
}

func (s *server) findNetworkInterfaceIndex(ip net.IP) (int, string, error) {
	var name string
	netInterfaces, err := net.Interfaces()
	if err != nil {
		log.Println("Failed to get network interfaces: ", err)
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

func (s *server) setLinkDNSAndRoutingDomain(address net.IP) (dbus.BusObject, error) {
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

	var linkPath dbus.ObjectPath
	var callFlags dbus.Flags

	manager := s.systemBus.Object(dbusResolveInterface, dbusResolvePath)
	err := manager.Call(dbusResolveGetLinkMethod, callFlags, s.linkIndex).Store(&linkPath)
	if err != nil {
		log.Println("Failed to get link: ", err)
		return nil, fmt.Errorf("failed to get link: %s", err)
	}

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

	// update link with new DNS server address
	link := s.systemBus.Object(dbusResolveInterface, linkPath)
	err = link.Call(dbusResolveSetDNSMethod, callFlags, addresses).Store()
	if err != nil {
		log.Println("Failed to set link DNS: ", err)
		return nil, fmt.Errorf("failed to set link DNS: %s", err)
	}

	var domains []Domain
	domains = append(domains, Domain{
		Name:    s.domainSuffix,
		Routing: true,
	})

	// update link with routing domain name
	err = link.Call(dbusResolveSetDomainsMethod, callFlags, domains).Store()
	if err != nil {
		log.Printf("Failed to set link Domain %s: %s\n", s.domainSuffix, err)
		return nil, fmt.Errorf("failed to set link Domain: %s", err)
	}

	return link, nil
}

func (s *server) runEventLoop() error {

	// channel for system interrupts
	interrupt := s.makeInterruptChannel()

	// channel for system events
	systemEvents, err := s.makeSystemEventsChannel()
	if err != nil {
		log.Println("Failed to setup DBus subscription: ", err)
		return err
	}

	for {
		select {
		case <-systemEvents:
			log.Println("Re-applying DNS change...")
			// re-apply DNS configuration after system resume
			err = s.applyDNSConfiguration()
			if err != nil {
				log.Println("Failed to apply DNS change: ", err)
				return err
			}
		case s := <-interrupt:
			log.Printf("Received %s signal\n", s.String())
			return nil
		}
	}
}

func (s *server) makeInterruptChannel() chan os.Signal {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	return c
}

func (s *server) makeSystemEventsChannel() (chan bool, error) {
	// see BecomeMonitor for interface details
	// https://dbus.freedesktop.org/doc/dbus-specification.html#bus-messages-become-monitor

	conn, err := dbus.SystemBus()
	if err != nil {
		log.Println("Failed to connect to system bus: ", err)
		return nil, fmt.Errorf("failed to connect to system bus: %s", err)
	}

	rules := []string{
		// match system suspend/resume via LoginManager
		signalMatchRule(
			matchInterface(dbusLoginManagerInterface),
			matchMember(dbusPrepareForSleepSignal)),

		// match changes to link via properties interface
		signalMatchRule(
			matchInterface(dbusPropertiesInterface),
			matchMember(dbusPropertiesChangedSignal),
			matchPath(dbusResolvePath)),
	}
	var flag uint = 0

	err = conn.BusObject().Call(dbusBecomeMonitorMethod, 0, rules, flag).Store()
	if err != nil {
		log.Println("Failed to become monitor: ", err)
		return nil, fmt.Errorf("failed to become monitor: %s", err)
	}

	bus := make(chan *dbus.Message, dbusChannelBufferSize)
	conn.Eavesdrop(bus)

	c := make(chan bool)

	go func() {
		for msg := range bus {

			// even though monitor rules are set, check for expected message
			// in testing sometimes different types of messages were delivered

			var ok bool
			var fieldPath, fieldInterface, fieldMember dbus.Variant

			if fieldPath, ok = msg.Headers[dbus.FieldPath]; !ok {
				goto done
			}

			if fieldInterface, ok = msg.Headers[dbus.FieldInterface]; !ok {
				goto done
			}

			if fieldMember, ok = msg.Headers[dbus.FieldMember]; !ok {
				goto done
			}

			// system resume message
			if fieldInterface.Value() == dbusLoginManagerInterface &&
				fieldMember.Value() == dbusPrepareForSleepSignal {

				// false when system resuming
				resuming := !msg.Body[0].(bool)
				if resuming {
					log.Println("System Resuming...")
					c <- true
				} else {
					log.Println("System Suspending...")
				}
				goto done
			}

			// network properties changed message
			if fieldInterface.Value() == dbusPropertiesInterface &&
				fieldMember.Value() == dbusPropertiesChangedSignal &&
				fieldPath.Value() == dbus.ObjectPath(dbusResolvePath) {

				//
				// this is(?) a bit of a hack... to determine whether the DNS configuration
				// on the network interface has been reverted, which seems to happen whenever
				// there is a change to any other network interface. E.g. unplugging the ethernet
				// cable or turning off WiFi causes our network configuration to be reverted.
				//
				// subscribing to PropertiesChanged on the org.freedesktop.DBus.Properties interface
				// lets us know when changes happen, and the logic checks whether our network interface
				// is missing from the list of DNS properties (on the respective link Index).
				//
				// https://dbus.freedesktop.org/doc/dbus-specification.html#standard-interfaces-properties
				//
				// message format:
				//
				//    org.freedesktop.DBus.Properties.PropertiesChanged (STRING interface_name,
				//                                                       DICT<STRING,VARIANT> changed_properties,
				//                                                       ARRAY<STRING> invalidated_properties);
				//

				var interfaceName string
				var changedProperties map[string]dbus.Variant
				var invalidatedProperties []string

				if err := dbus.Store(msg.Body, &interfaceName, &changedProperties, &invalidatedProperties); err != nil {
					log.Println("Failed to unpack properties changed message: ", err)
					goto done
				}

				// double check...
				if interfaceName != dbusResolveManagerInterface {
					//log.Println("Error wrong interface ", interfaceName, ", expecting ", dbusResolveManagerInterface)
					goto done
				}

				// only interested in the DNS property
				if dnsPropList, ok := changedProperties["DNS"]; ok {

					// property is @a(iiay)
					dnsProps := dnsPropList.Value().([][]interface{})
					for _, dnsProp := range dnsProps {

						var linkIndex int32
						var addressFamily int32
						var ipAddress []uint8

						if err := dbus.Store(dnsProp, &linkIndex, &addressFamily, &ipAddress); err != nil {
							log.Println("Failed to unpack DNS properties: ", err)
							goto done
						}

						// matches?
						if linkIndex == int32(s.linkIndex) {
							goto done
						}
					}

					log.Println("Network change detected!")
					c <- true
				}
			}

		done:
		}
	}()

	return c, nil
}

func (s *server) revertDNSForLink() error {
	// see LinkObject for interface details
	// https://www.freedesktop.org/wiki/Software/systemd/resolved/

	if s.linkObject == nil {
		return nil
	}

	var callFlags dbus.Flags
	err := s.linkObject.Call("org.freedesktop.resolve1.Link.Revert", callFlags).Store()
	if err != nil {
		log.Println("Failed to revert link DNS: ", err)
		return fmt.Errorf("failed to revert link DNS: %s", err)
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
		log.Printf("Failed to stop DNS container %s: %s\n", s.dnsContainer.ID, err)
		return fmt.Errorf("failed to stop DNS container: %s", err)
	}

	return nil
}

type matchOption struct {
	key   string
	value string
}

func matchType(value string) matchOption {
	return matchOption{
		key:   "type",
		value: value,
	}
}

func matchPath(value string) matchOption {
	return matchOption{
		key:   "path",
		value: value,
	}
}

func matchInterface(value string) matchOption {
	return matchOption{
		key:   "interface",
		value: value,
	}
}

func matchMember(value string) matchOption {
	return matchOption{
		key:   "member",
		value: value,
	}
}

func signalMatchRule(options ...matchOption) string {
	options = append([]matchOption{matchType("signal")}, options...)
	items := make([]string, 0, len(options))
	for _, option := range options {
		items = append(items, option.key+"='"+option.value+"'")
	}
	return strings.Join(items, ",")
}
