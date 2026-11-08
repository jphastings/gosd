package main

import (
	"github.com/jphastings/gosd/internal/jetsonflash"
	"github.com/spf13/cobra"
)

func newSpikeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "spike <signed-files-dir>",
		Short:  "Experimental: boot a module in recovery mode into CBoot's flashing server with NVIDIA-signed files",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return jetsonflash.RunSpike(args[0], cmd.OutOrStdout())
		},
	}
	return cmd
}
