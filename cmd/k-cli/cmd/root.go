package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "k-cli",
	Short: "CLI to test the Kairos P2P engine",
	Long:  "CLI to interact directly with Kairos network P2P nodes without a Gateway",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
