package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"go.virtualstaticvoid.com/ldhdns/internal/dns"
)

func init() { Root.AddCommand(NewCmdDns()) }

// NewCmdDns creates a new cobra.Command for the dns sub-command.
func NewCmdDns() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dns",
		Short: "Runs ldhdns in DNS mode",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := dns.Run(domainSuffix, subDomainLabel, dnsmasqHostsDirectory, dnsmasqPidFile); err != nil {
				log.Fatal(err)
			}
		},
	}

	cmd.Flags().StringVar(
		&domainSuffix,
		"domain-suffix",
		defaultDomainSuffix,
		"Domain name suffix for DNS resolution.")

	cmd.Flags().StringVar(
		&subDomainLabel,
		"subdomain-label",
		defaultSubDomainLabel,
		"Name of the label used to provide the sub-domain of a container.")

	cmd.Flags().StringVar(
		&dnsmasqHostsDirectory,
		"dnsmasq-hostsdir",
		defaultDnsmasqHostsDirectory,
		"Directory for host entries to be written to which dnsmasq will read.")

	cmd.Flags().StringVar(
		&dnsmasqPidFile,
		"dnsmasq-pidfile",
		defaultDnsmasqPidFile,
		"PID file of the dnsmasq process.")

	return cmd
}
