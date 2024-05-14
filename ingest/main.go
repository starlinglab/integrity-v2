package main

import (
	"fmt"
	"ingest-v2/utils"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.MaxMultipartMemory = 1024 << 20 // 1 GB

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	r.POST("/upload", func(c *gin.Context) {
		// Multipart form
		form, _ := c.MultipartForm()
		files := form.File["file"]

		var results [][]string
		for _, file := range files {
			log.Println(file.Filename)

			openFile, err := file.Open()
			if err != nil {
				log.Println(err)
				return
			}
			cid := utils.Cid(openFile)
			results = append(results, []string{file.Filename, cid})
		}

		var cidPairs []string
		for _, pair := range results {
			joinedPair := strings.Join(pair, ":")
			cidPairs = append(cidPairs, joinedPair)
		}

		cidOutputString := strings.Join(cidPairs, "\n")
		c.String(http.StatusOK,
			fmt.Sprintf("%d files uploaded!\n%s", len(files), cidOutputString),
		)
	})

	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
