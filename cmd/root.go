package cmd

import (
	"github.com/spf13/cobra"
)

const (
	// configuration defaults
	defaultNetworkId             = "ldhdns"
	defaultDomainSuffix          = "ldh.dns"
	defaultSubDomainLabel        = "dns.ldh/subdomain"
	defaultDnsmasqHostsDirectory = "/etc/ldhdns/dnsmasq/hosts.d"
	defaultDnsmasqPidFile        = "/var/run/dnsmasq.pid"
	defaultContainerName         = "ldhdns"
)

var (
	// configuration variables
	networkId             string
	domainSuffix          string
	subDomainLabel        string
	dnsmasqHostsDirectory string
	dnsmasqPidFile        string
	containerName         string

	// Version can be set via:
	// -ldflags="-X go.virtualstaticvoid.com/ldhdns/cmd.Version=$VERSION"
	Version string

	// top-level cobra.Command
	Root = &cobra.Command{
		Use:   "ldhdns",
		Short: "a tool to provide DNS for docker containers running on a single host.",
		Run:   func(cmd *cobra.Command, _ []string) { cmd.Usage() },
	}
)
