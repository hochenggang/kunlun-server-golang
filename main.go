package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	host := os.Getenv("KUNLUN_HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("KUNLUN_PORT")
	if port == "" {
		port = "8008"
	}

	r := setupRouter()
	addr := fmt.Sprintf("%s:%s", host, port)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
