package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	listen := getenv("LISTEN", "0.0.0.0:7505")
	agentURL := getenv("AGENT_URL", "http://127.0.0.1:7506")
	server := &Server{
		store:    NewStore(),
		agentURL: agentURL,
		token:    os.Getenv("API_TOKEN"),
		client:   &http.Client{Timeout: 180 * time.Second},
	}
	log.Printf("laya gateway listening on http://%s (brain %s)", listen, agentURL)
	if err := http.ListenAndServe(listen, server.routes()); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
