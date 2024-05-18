package main

import (
	"encoding/json"
	"ingest-v2/utils"
	"io"
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

	r.GET("/c/:cid", func(c *gin.Context) {
		cid := c.Param("cid")
		v, err := utils.GetAllAttributes(cid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, utils.CastMapForJSON(v))
	})

	r.GET("/c/:cid/:attr", func(c *gin.Context) {
		cid := c.Param("cid")
		attr := c.Param("attr")
		v, err := utils.GetAttribute(cid, attr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, utils.CastMapForJSON(v))
	})

	r.POST("/upload", func(c *gin.Context) {
		// Multipart form
		form, _ := c.MultipartForm()
		files := form.File["file"]

		if len(files) != 1 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Please upload only one file as 'file'.",
			})
			return
		}
		file, err := files[0].Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
		defer file.Close()
		cid := utils.Cid(file)

		metadatas := form.File["metadata"]
		if len(metadatas) != 1 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Please upload only one metadata file as 'metadata'.",
			})
			return
		}
		metadataFile, err := metadatas[0].Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
		defer metadataFile.Close()
		metadataString, err := io.ReadAll(metadataFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		var jsonMap interface{}
		err = json.Unmarshal(metadataString, &jsonMap)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
		attributes := utils.ParseJsonToAttributes(jsonMap)
		utils.PostNewAttribute(cid, attributes)

		c.JSON(http.StatusOK, gin.H{
			"cid": cid,
		})
	})

	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
