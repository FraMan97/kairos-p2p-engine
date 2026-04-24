package cmd

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/FraMan97/kairos-p2p-engine/cmd/k-cli/config"
	"github.com/spf13/cobra"
)

var filePath string
var releaseTime string

var putCmd = &cobra.Command{
	Use:   "put",
	Short: "Upload a file to the P2P network",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Uploading file %s ...\n", filePath)
		file, err := os.Open(filePath)
		if err != nil {
			log.Println("Error opening file: ", err)
			return
		}
		defer file.Close()

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			log.Println("Error creating form file:", err)
			return
		}

		if _, err = io.Copy(part, file); err != nil {
			log.Println("Error copying file content:", err)
			return
		}

		if err = writer.WriteField("release_time", releaseTime); err != nil {
			log.Println("Error writing release_time field:", err)
			return
		}

		if err = writer.Close(); err != nil {
			log.Println("Error closing writer:", err)
			return
		}

		targetURL := fmt.Sprintf("%s/put", config.NodeURL)
		resp, err := http.Post(targetURL, writer.FormDataContentType(), body)
		if err != nil {
			log.Println("Error calling put endpoint: ", err)
			return
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 200 {
			log.Printf("File uploaded successfully! ID: %s\n", string(bodyBytes))
		} else {
			log.Printf("Error uploading file (status %d): %s\n", resp.StatusCode, string(bodyBytes))
		}
	},
}

func init() {
	rootCmd.AddCommand(putCmd)
	putCmd.Flags().StringVarP(&filePath, "file-path", "f", "", "Path of the file to upload")
	putCmd.Flags().StringVarP(&releaseTime, "release-time", "r", "", "Release date (e.g., 2026-12-01T15:00:00Z)")
	putCmd.MarkFlagRequired("file-path")
	putCmd.MarkFlagRequired("release-time")
}
