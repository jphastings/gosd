package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/docview"
)

func newDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "docs [name]",
		Short:     "Read gosd's documentation",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: docview.Names(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				for _, name := range docview.Names() {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), name)
				}
				return nil
			}
			return docview.Show(args[0], cmd.OutOrStdout())
		},
	}
}
