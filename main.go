package main

import (
	"log"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	r := setupRouter()
	if err := r.Run("0.0.0.0:8008"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
