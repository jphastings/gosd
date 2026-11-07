package main

import (
	"github.com/jphastings/gosd/internal/jetsonflash"
	"github.com/spf13/cobra"
)

func newSpikeCmd() *cobra.Command {
	var o jetsonflash.SpikeOptions
	cmd := &cobra.Command{
		Use:    "spike <signed-files-dir>",
		Short:  "Experimental: boot a module in recovery mode into CBoot's flashing server with NVIDIA-signed files",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return jetsonflash.RunSpike(args[0], o, cmd.OutOrStdout())
		},
	}
	cmd.Flags().Uint32Var(&o.VersionCmd, "version-cmd", 0, "nv3p command each session opens with (0: GET_PLATFORM_INFO)")
	cmd.Flags().Uint32Var(&o.BootCmd, "boot-cmd", 0, "nv3p command that boots CBoot's flashing server (0: stop after rp1)")
	return cmd
}
