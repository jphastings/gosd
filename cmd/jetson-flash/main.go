// Command jetson-flash flashes a gosd-built image onto a Jetson Nano eMMC
// module over USB recovery mode, without NVIDIA's host tools.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "jetson-flash",
		Short:         "Flash a gosd-built image onto a Jetson Nano eMMC module over USB recovery mode",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newSpikeCmd())
	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "jetson-flash:", err)
		os.Exit(1)
	}
}
