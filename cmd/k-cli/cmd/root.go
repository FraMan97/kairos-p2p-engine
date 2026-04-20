package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "k-cli",
	Short: "CLI per testare il motore P2P Kairos",
	Long:  "CLI per interagire direttamente con i nodi P2P della rete Kairos senza Gateway",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
