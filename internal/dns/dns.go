package dns

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type server struct {
	lock           sync.RWMutex
	docker         *client.Client
	ctx            context.Context
	domainSuffix   string
	subDomainLabel string
	hostsPath      string
	pidFile        string
}

func Run(domainSuffix string, subDomainLabel string, hostsPath string, pidFile string) error {
	log.Println("Starting...")
	server, err := newServer(domainSuffix, subDomainLabel, hostsPath, pidFile)
	if err != nil {
		log.Println("Failed to start server: ", err)
		return err
	}

	log.Printf("Configured for %q domain and %q container label.\n", domainSuffix, subDomainLabel)

	log.Println("Loading existing containers...")
	err = server.loadRunningContainers()
	if err != nil {
		log.Println("Failed to load existing containers: ", err)
		return err
	}

	log.Println("Running event loop...")
	err = server.runEventLoop()
	if err != nil {
		log.Println("Failed to run event loop: ", err)
		return err
	}

	log.Println("Shutting down...")
	err = server.close()
	if err != nil {
		log.Println("Failed shutdown: ", err)
		return err
	}

	log.Println("Bye...")
	return nil
}

func newServer(domainSuffix string, subDomainLabel string, hostsPath string, pidFile string) (*server, error) {
	// connect to the docker API - uses DOCKER_HOST environment variable
	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Println("Failed to connect docker client: ", err)
		return nil, err
	}

	// context for background processing
	// which traps SIGINT/SIGTERM
	ctx := contextWithSignal(context.Background())

	// create variable to hold server state
	return &server{
		docker:         docker,
		ctx:            ctx,
		domainSuffix:   domainSuffix,
		subDomainLabel: subDomainLabel,
		hostsPath:      hostsPath,
		pidFile:        pidFile,
	}, nil
}

func (s *server) close() error {
	return s.docker.Close()
}

func (s *server) loadRunningContainers() error {
	containerList, err := s.docker.ContainerList(s.ctx, types.ContainerListOptions{})
	if err != nil {
		log.Println("Error listing containers: ", err)
		return err
	}

	for _, container := range containerList {
		err = s.containerAdded(container.ID)
		if err != nil {
			log.Printf("[%s] Error loading container: %s\n", container.ID, err)
			return err
		}
	}

	return nil
}

func (s *server) runEventLoop() error {
	// we're only interested in container events
	filter := filters.NewArgs()
	filter.Add("type", events.ContainerEventType)

	// open docker event stream
	eventsChan, errorsChan := s.docker.Events(s.ctx, types.EventsOptions{Filters: filter})

	// make channel for capturing returned value
	// also provides elegant means for waiting on the event loop
	result := make(chan error)

	go func() {
		for {
			select {
			case event := <-eventsChan:
				if err := s.handleDockerEvent(event); err != nil {
					log.Println("Event read failed: ", err)
					result <- err
					return
				}
			case err := <-errorsChan:
				if err == io.EOF {
					log.Println("Event loop shutting down")
					result <- nil
				} else {
					log.Println("Event loop shutting down: ", err)
					result <- err
				}
				return
			}
		}
	}()

	// wait here
	return <-result
}

func (s *server) handleDockerEvent(event events.Message) error {
	switch event.Action {
	case "start":
		return s.containerAdded(event.ID)
	case "stop", "die":
		return s.containerRemoved(event.ID)
	}
	return nil
}

func (s *server) containerAdded(containerID string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	log.Printf("Examining container %s\n", containerID)

	// get container metadata
	meta, err := s.docker.ContainerInspect(s.ctx, containerID)
	if err != nil {
		log.Printf("[%s] Error inspecting container: %s\n", containerID, err)
		return err
	}

	// obviously
	if !meta.State.Running {
		return nil
	}

	// look for special host label
	subDomain := meta.Config.Labels[s.subDomainLabel]
	if len(subDomain) == 0 {
		return nil
	}

	// append domain
	hostName := fmt.Sprintf("%s.%s", subDomain, s.domainSuffix)

	// write "DNS" host file
	file, err := os.Create(filepath.Join(s.hostsPath, containerID))
	if err != nil {
		log.Println("Error creating file: ", err)
		return err
	}
	defer file.Close()

	log.Printf("Registering %q\n", hostName)

	for _, containerNetwork := range meta.NetworkSettings.Networks {
		// IPv4 address
		log.Printf(" → IPv4Address: %q\n", containerNetwork.IPAddress)
		_, err = fmt.Fprintf(file, "%s\t%s\n", containerNetwork.IPAddress, hostName)
		if err != nil {
			log.Println("Error writing file (IPv4): ", err)
			return err
		}

		// IPv6 address
		if len(containerNetwork.GlobalIPv6Address) > 0 {
			log.Printf(" → IPv6Address: %q\n", containerNetwork.GlobalIPv6Address)
			_, err = fmt.Fprintf(file, "%s\t%s\n", containerNetwork.GlobalIPv6Address, hostName)
			if err != nil {
				log.Println("Error writing file (IPv6): ", err)
				return err
			}
		}
	}

	return nil
}

func (s *server) containerRemoved(containerID string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// TODO: queue up container removals so if multiple containers
	// are terminating at the same time we don't unnecessarily
	// signal dnsmasq to reload it's configuration

	fileName := filepath.Join(s.hostsPath, containerID)

	// file exists?
	_, err := os.Stat(fileName)
	if err != nil {
		// ignore error
		return nil
	}

	// delete the file
	err = os.Remove(fileName)
	if err != nil {
		// ignore error
		return nil
	}

	// SIGHUP to reload config
	return s.signalDnsmasq()
}

func (s *server) signalDnsmasq() error {
	pid, err := s.readDnsmasqPID()
	if err != nil {
		return err
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		log.Printf("Error finding dnsmasq process [PID: %d]: %s\n", pid, err)
		return err
	}

	err = process.Signal(syscall.SIGHUP)
	if err != nil {
		log.Printf("Error signalling dnsmasq process [PID: %d]: %s\n", pid, err)
		return err
	}

	return nil
}

func (s *server) readDnsmasqPID() (int, error) {
	contents, err := ioutil.ReadFile(s.pidFile)
	if err != nil {
		log.Printf("Error reading dnsmasq PID file %q: %s\n", s.pidFile, err)
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		log.Printf("Invalid dnsmasq PID %q: %s\n", string(contents), err)
		return 0, err
	}

	return pid, nil
}

// helper functions

func contextWithSignal(ctx context.Context) context.Context {
	newCtx, cancel := context.WithCancel(ctx)
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-signals:
			log.Printf("Received %s signal\n", sig.String())
			cancel()
		}
	}()
	return newCtx
}
