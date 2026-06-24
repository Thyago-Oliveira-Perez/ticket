package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type client struct {
	ID	string `json:"id"`
	Name	string `json:"name`
}

var clients = []client {
	{ID: "1", Name: "Thyago"},
	{ID: "2", Name: "Monica"},
}

func main() {
	router := gin.Default()
	router.GET("/clients", getClients)

	router.Run("localhost:8080")
}

func getClients(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, clients)
}