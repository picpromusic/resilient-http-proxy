package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Configuration
const (
	maxRetries              = 120         // Number of retry attempts
	retryDelay              = time.Second // Delay between retries
	TRUE_OR_SIMULATED_FALSE = true
)

// Proxy handler with Accept-Ranges validation
func proxyHandler(w http.ResponseWriter, r *http.Request, upstream string) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Connection hijacking not supported", http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodGet {
		err := resilientGet(r, upstream, hj, w)
		if err != nil {
			log.Printf("Error in proxyHandler: %v\n", err)
		}
	} else {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	// Parse CLI arguments for port and upstream URL
	port := flag.Int("port", 3000, "Port to run the proxy server on")
	upstream := flag.String("upstream", "", "Upstream server URL")
	flag.Parse()

	// Print startup information
	log.Printf("Starting retry proxy server...\n")
	log.Printf("Upstream server: %s\n", *upstream)
	log.Printf("Listening on port: %d\n", *port)

	// Set the upstream URL globally
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		proxyHandler(w, r, *upstream)
	})

	log.Printf("Retry proxy server is running on http://localhost:%d\n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
