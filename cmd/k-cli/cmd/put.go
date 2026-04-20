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
	Short: "Invia un file alla rete P2P",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Invio del file %s ...\n", filePath)
		file, err := os.Open(filePath)
		if err != nil {
			log.Println("Errore apertura file: ", err)
			return
		}
		defer file.Close()

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			log.Println("Errore creazione form file:", err)
			return
		}

		if _, err = io.Copy(part, file); err != nil {
			log.Println("Errore copia contenuto file:", err)
			return
		}

		if err = writer.WriteField("release_time", releaseTime); err != nil {
			log.Println("Errore scrittura release_time:", err)
			return
		}

		if err = writer.Close(); err != nil {
			log.Println("Errore chiusura writer:", err)
			return
		}

		targetURL := fmt.Sprintf("%s/put", config.NodeURL)
		resp, err := http.Post(targetURL, writer.FormDataContentType(), body)
		if err != nil {
			log.Println("Errore chiamata endpoint put: ", err)
			return
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if resp.StatusCode == 200 {
			log.Printf("File inviato con successo! ID: %s\n", string(bodyBytes))
		} else {
			log.Printf("Errore invio file (status %d): %s\n", resp.StatusCode, string(bodyBytes))
		}
	},
}

func init() {
	rootCmd.AddCommand(putCmd)
	putCmd.Flags().StringVarP(&filePath, "file-path", "f", "", "Percorso del file da caricare")
	putCmd.Flags().StringVarP(&releaseTime, "release-time", "r", "", "Data di rilascio (es. 2026-12-01T15:00:00Z)")
	putCmd.MarkFlagRequired("file-path")
	putCmd.MarkFlagRequired("release-time")
}
