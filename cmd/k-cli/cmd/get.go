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
	Short: "Scarica un file tramite fileId",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Richiesta file %s alla rete...", fileId)

		targetURL := fmt.Sprintf("%s/get?id=%s", config.NodeURL, fileId)
		resp, err := http.Get(targetURL)
		if err != nil {
			log.Println("Errore chiamata get endpoint: ", err)
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
				log.Println("Errore creazione file di output: ", err)
				return
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, resp.Body); err != nil {
				log.Println("Errore salvataggio file: ", err)
				return
			}

			log.Printf("File ricostruito con successo salvato in: %s\n", outPath)
		} else {
			bodyBytes, _ := io.ReadAll(resp.Body)
			log.Printf("Errore dal server (status %d): %s\n", resp.StatusCode, string(bodyBytes))
		}
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.Flags().StringVarP(&fileId, "file-id", "f", "", "ID del file da scaricare")
	getCmd.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "Cartella di destinazione")
	getCmd.MarkFlagRequired("file-id")
}
