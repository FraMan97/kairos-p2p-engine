package cmd

import (
	"fmt"
	"log"
	"net/http"

	"github.com/FraMan97/kairos-p2p-engine/cmd/k-cli/config"
	"github.com/spf13/cobra"
)

var deleteFileId string

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a file and all its shards from the P2P network",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Deletion request for file %s ...\n", deleteFileId)

		targetURL := fmt.Sprintf("%s/delete?id=%s", config.NodeURL, deleteFileId)
		req, err := http.NewRequest(http.MethodDelete, targetURL, nil)
		if err != nil {
			log.Println("Error creating request: ", err)
			return
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Println("Error calling delete endpoint: ", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Println("File successfully deleted from the network!")
		} else {
			log.Printf("Error deleting file (status %d)\n", resp.StatusCode)
		}
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().StringVarP(&deleteFileId, "file-id", "f", "", "ID of the file to delete")
	deleteCmd.MarkFlagRequired("file-id")
}
