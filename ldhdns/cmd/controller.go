package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"go.virtualstaticvoid.com/ldhdns/pkg/controller"
)

func init() { Root.AddCommand(NewCmdController()) }

// NewCmdController creates a new cobra.Command for the controller sub-command.
func NewCmdController() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Runs ldhdns in controller mode",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, args []string) {
			if err := controller.Run(domainSuffix); err != nil {
				log.Fatal(err)
			}
		},
	}

	cmd.Flags().StringVar(
		&domainSuffix,
		"domain-suffix",
		defaultDomainSuffix,
		"Domain name suffix for DNS resolution.")

	return cmd
}
