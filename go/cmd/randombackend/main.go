package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var disableRanges bool
var waitEveryNElements int
var randomEtag bool
var currentModified bool

// RangeRequestHandler handles GET requests with range support
func RangeRequestHandler(w http.ResponseWriter, r *http.Request) {

	for k, v := range r.Header {
		fmt.Printf("> %s: %s\n", k, v)
	}

	// Parse the URL path
	path := r.URL.Path
	if !strings.HasPrefix(path, "/generate/") {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Extract the number of bytes from the URL
	numBytesStr := strings.TrimPrefix(path, "/generate/")
	numBytes, err := strconv.Atoi(numBytesStr)
	if err != nil || numBytes < 0 {
		http.Error(w, "Invalid number of bytes", http.StatusBadRequest)
		return
	}

	start, end := 0, numBytes-1 // Default range
	rangeHeader := ""
	if !disableRanges {
		// Handle Range header
		rangeHeader = r.Header.Get("Range")
		if rangeHeader != "" {
			rangeValue := strings.TrimPrefix(rangeHeader, "bytes=")
			rangeParts := strings.Split(rangeValue, "-")
			if len(rangeParts) != 2 {
				http.Error(w, "Invalid Range Header", http.StatusBadRequest)
				return
			}

			// Parse start and end values
			start, err = strconv.Atoi(rangeParts[0])
			if err != nil || start < 0 {
				http.Error(w, "Invalid Range Header", http.StatusBadRequest)
				return
			}
			if rangeParts[1] != "" {
				end, err = strconv.Atoi(rangeParts[1])
				if err != nil || end >= numBytes || start > end {
					http.Error(w, "Requested Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
					return
				}
			} else {
				end = numBytes - 1
			}
		}
	}

	// Send response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
	if randomEtag {
		etag := fmt.Sprintf("%x", rand.Int63())
		w.Header().Set("ETag", etag)
	} else {
		w.Header().Set("ETag", numBytesStr)
	}

	log.Printf("CurrentModified: %v", currentModified)

	if currentModified {
		w.Header().Set("Last-Modified", time.Now().Format(http.TimeFormat))
	} else {
		w.Header().Set("Last-Modified", time.Unix(0, 0).Format(http.TimeFormat))
	}

	if !disableRanges && rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, numBytes))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		if !disableRanges {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		w.WriteHeader(http.StatusOK)
	}
	// iterate over header and print line by line
	for k, v := range w.Header() {
		fmt.Printf("< %s: %s\n", k, v)
	}

	// Generate the requested range of random bytes using a local random generator
	rng := rand.New(rand.NewSource(0)) // Fixed seed for reproducibility

	if start > 0 {
		fmt.Printf("Skipping %d  random bytes\n", start)
	}

	for i := 0; i < start; i++ {
		rng.Intn(256)
	}

	chunkSize := 1024 * 1024
	if waitEveryNElements > 0 {
		chunkSize = waitEveryNElements
	}

	data := make([]byte, chunkSize)

	for sent := start; sent <= end; {
		remaining := end - sent + 1
		toSend := chunkSize
		if remaining < chunkSize {
			toSend = remaining
		}

		for i := 0; i < toSend; i++ {
			data[i] = byte(rng.Intn(256))
		}

		// Send the chunk
		w.Write(data[:toSend])
		sent += toSend
		if waitEveryNElements > 0 {
			// Simulate a delay every N elements
			fmt.Printf("Waiting for %d random bytes\n", waitEveryNElements)
			time.Sleep(1 * time.Second)
		}

	}
}

func main() {
	// Parse command-line arguments for port
	port := flag.Int("port", 5000, "Port to run the server on (default: 5000)")
	randomEtagPtr := flag.Bool("randomEtag", false, "Generate a random ETag for each request")
	currentModifiedPtr := flag.Bool("currentModified", false, "Set Last-Modified to current time")
	disableRangesPtr := flag.Bool("disableRanges", false, "Disable range requests")
	waitEveryNElementsPtr := flag.Int("waitEveryNElements", 0, "Wait a second every N elements (default: 0, no wait)")

	// Parse the flags
	flag.Parse()

	// Assign parsed values to global variables
	disableRanges = *disableRangesPtr
	waitEveryNElements = *waitEveryNElementsPtr
	randomEtag = *randomEtagPtr
	currentModified = *currentModifiedPtr // Ensure this is set correctly

	// Log the parsed values for debugging
	log.Printf("Parsed Flags - disableRanges: %v, waitEveryNElements: %d, randomEtag: %v, currentModified: %v", disableRanges, waitEveryNElements, randomEtag, currentModified)

	http.HandleFunc("/generate/", RangeRequestHandler)

	// Start the server
	log.Printf("Starting server on port %d...", *port)
	if disableRanges {
		log.Printf("Range Request not supported")
	} else {
		log.Printf("Range Request are supported")
	}
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
