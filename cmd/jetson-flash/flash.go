package main

import (
	"io"

	"github.com/jphastings/gosd/internal/jetsonflash"
	"github.com/spf13/cobra"
)

func newFlashCmd() *cobra.Command {
	var trace bool
	cmd := &cobra.Command{
		Use:   "flash [flags] <image.img> <signed-files-dir>",
		Short: "Rewrite a module's APP partition with a gosd image, over USB recovery mode",
		Long: `Rewrites only the APP partition, so the module must already carry NVIDIA's partition layout from one earlier flash by NVIDIA's tools. It must be waiting in USB recovery mode.

<signed-files-dir> is the unpacked massflash blob from that earlier flash.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var log io.Writer
			if trace {
				log = cmd.OutOrStdout()
			}
			return jetsonflash.RunFlash(args[0], args[1], cmd.OutOrStdout(), log)
		},
	}
	cmd.Flags().BoolVar(&trace, "trace", false, "log every USB transfer and nv3p packet")
	return cmd
}
