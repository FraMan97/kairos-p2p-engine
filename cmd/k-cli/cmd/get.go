package cmd

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/FraMan97/kairos-p2p-engine/cmd/k-cli/config"
	"github.com/spf13/cobra"
)

var fileId string
var outputDir string

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Download a file via fileId",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Requesting file %s from the network...", fileId)

		targetURL := fmt.Sprintf("%s/get?id=%s", config.NodeURL, fileId)
		resp, err := http.Get(targetURL)
		if err != nil {
			log.Println("Error calling get endpoint: ", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			filename := "downloaded_" + fileId
			contentDisp := resp.Header.Get("Content-Disposition")
			if contentDisp != "" {
				fmt.Sscanf(contentDisp, "attachment; filename=\"%s\"", &filename)
				filename = filename[:len(filename)-1]
			}

			outPath := fmt.Sprintf("%s/%s", outputDir, filename)
			outFile, err := os.Create(outPath)
			if err != nil {
				log.Println("Error creating output file: ", err)
				return
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, resp.Body); err != nil {
				log.Println("Error saving file: ", err)
				return
			}

			log.Printf("File reconstructed successfully and saved to: %s\n", outPath)
		} else {
			bodyBytes, _ := io.ReadAll(resp.Body)
			log.Printf("Server error (status %d): %s\n", resp.StatusCode, string(bodyBytes))
		}
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.Flags().StringVarP(&fileId, "file-id", "f", "", "ID of the file to download")
	getCmd.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "Destination directory")
	getCmd.MarkFlagRequired("file-id")
}
