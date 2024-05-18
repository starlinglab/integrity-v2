package main

import (
	"encoding/json"
	"ingest-v2/utils"
	"io"
	"log"
	"net/http"

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
		file, err := files[0].Open()
		if err != nil {
			log.Println(err)
			return
		}
		cid := utils.Cid(file)

		metadatas := form.File["metadata"]
		metadataFile, err := metadatas[0].Open()
		if err != nil {
			log.Println(err)
			return
		}
		metadataString, err := io.ReadAll(metadataFile)
		if err != nil {
			log.Println(err)
			return
		}

		var jsonMap interface{}
		err = json.Unmarshal(metadataString, &jsonMap)
		if err != nil {
			log.Printf("ERROR: fail to unmarshal json, %s", err.Error())
		}
		attributes := utils.ParseJsonToAttributes(jsonMap)
		utils.PostNewAttribute(cid, attributes)

		c.JSON(http.StatusOK, gin.H{
			"cid": cid,
		})
	})

	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
