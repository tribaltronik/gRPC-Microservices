//go:build healthcheck

package main

import (
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://localhost:50053/healthz")
	if err != nil {
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
