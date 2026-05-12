package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "grpc-poc-api-key-2026"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "50053"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-api-key")
		if key == "" {
			http.Error(w, `{"error":"missing x-api-key header"}`, http.StatusUnauthorized)
			return
		}
		if key != apiKey {
			http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"authorized"}`)
	})

	log.Printf("starting ext-authz on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
