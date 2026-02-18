package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"clawtivity/internal/database"
)

type Server struct {
	port int

	db database.Service
}

func NewServer() *http.Server {
	port := resolvePort()
	NewServer := &Server{
		port: port,

		db: database.New(),
	}

	// Declare Server config
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", NewServer.port),
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return server
}

func resolvePort() int {
	value := os.Getenv("PORT")
	if value == "" {
		return 18730
	}

	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 {
		return 18730
	}

	return port
}
