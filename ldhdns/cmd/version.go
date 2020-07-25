package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func init() { Root.AddCommand(NewCmdVersion()) }

// NewCmdVersion creates a new cobra.Command for the version sub-command.
func NewCmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints the version",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, args []string) {
			if Version == "" {
				buildInfo, ok := debug.ReadBuildInfo()
				if !ok {
					fmt.Println("could not determine build information")
					return
				}
				Version = buildInfo.Main.Version
			}
			fmt.Println(Version)
		},
	}
}
