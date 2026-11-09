package main

import (
	"os"

	"github.com/jphastings/gosd/internal/jetsonflash"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newFlashCmd() *cobra.Command {
	var o jetsonflash.FlashOptions
	var trace bool
	cmd := &cobra.Command{
		Use:   "flash [flags] <image.img>",
		Short: "Flash a gosd image onto a Jetson Nano eMMC module in USB recovery mode",
		Long:  "Erases the module's eMMC, then writes NVIDIA's boot chain, built from NVIDIA's L4T kit, with the image's boot partition as APP.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if trace {
				o.Trace = cmd.OutOrStdout()
			}
			if term.IsTerminal(int(os.Stdin.Fd())) {
				o.Terminal = cmd.InOrStdin()
			}
			return jetsonflash.RunFlash(args[0], o, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&o.Kit, "kit", "", "read NVIDIA's L4T kit (Jetson-210_Linux_R32.7.6_aarch64.tbz2) from this file instead of downloading it")
	cmd.Flags().BoolVar(&o.AcceptLicense, "accept-nvidia-license", false, "accept NVIDIA's licence for the L4T kit without being asked")
	cmd.Flags().BoolVar(&trace, "trace", false, "log every USB transfer and nv3p packet")
	return cmd
}
