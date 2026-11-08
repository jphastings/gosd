package main

import (
	"time"

	"github.com/jphastings/gosd/internal/jetsonflash"
	"github.com/spf13/cobra"
)

func newProbeCmd() *cobra.Command {
	var data, out string
	var wait time.Duration
	cmd := &cobra.Command{
		Use:    "probe [flags] <spec>...",
		Short:  "Experimental: send commands to a running CBoot flashing server and record the replies",
		Long:   "Each spec is CMD, CMD:HEXARGS (e.g. 0x0b:41505000) or a range FIRST-LAST (e.g. 0x02-0x30) sent with empty args. Each gets its own session.",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return jetsonflash.RunProbe(args, jetsonflash.ProbeOptions{Wait: wait, DataFile: data, OutFile: out}, cmd.OutOrStdout())
		},
	}
	cmd.Flags().DurationVar(&wait, "wait", 10*time.Second, "how long to wait for the device's first reply to each command")
	cmd.Flags().StringVar(&data, "data", "", "file to send as DATA after each command's first reply")
	cmd.Flags().StringVar(&out, "out", "", "file to write every DATA payload the device sends (after the version query) to")
	return cmd
}
