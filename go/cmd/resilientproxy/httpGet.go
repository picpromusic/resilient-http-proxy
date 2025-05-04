package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Retry logic with range support
func fetchWithRetry(baseURL, verb string, path string, retries int, rangeHeader string) (*http.Response, error) {
	var lastErr error
	fullURL := fmt.Sprintf("%s%s", baseURL, path) // Append the requested path to the upstream URL
	logUpstream("Fetching URL: %s\n", fullURL)
	for attempt := 1; attempt <= retries; attempt++ {
		req, err := http.NewRequest(verb, fullURL, nil)
		if err != nil {
			return nil, err
		}

		// Add Range header if provided
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // Disable certificate verification
				},
			},
		}
		resp, err := client.Do(req)
		if err == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent) {
			// Log headers line by line
			for key, values := range resp.Header {
				logUpstream("%s: %s\n", key, values)
			}
			return resp, nil
		} else if err == nil && resp.StatusCode >= 400 && resp.StatusCode < 550 {
			return resp, nil
		}
		// Handle client errors (4xx) and server errors (5xx
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("upstream server returned status: %d", resp.StatusCode)
			if resp != nil {
				resp.Body.Close()
			}
		}

		if attempt < retries {
			sleepTime := min(60, (attempt)*(attempt))
			logUpstream("Retrying in %d seconds\n", sleepTime)
			time.Sleep(retryDelay * time.Duration(sleepTime))
			logUpstream("Retrying... (%d/%d)\n", attempt, retries)
			continue
		}
	}
	return nil, lastErr
}

// Check the client's Range request and determine if ranges are supported by the upstream server
func checkClientRangeRequest(r *http.Request, bytesSent *int64, end *int64, length *int64, savedETag *string, savedLastModified *string, upstream string) (bool, error) {

	lengthHeader := r.Header.Get("Content-Length")
	if lengthHeader != "" {
		var err error
		*length, err = strconv.ParseInt(lengthHeader, 10, 64)
		if err != nil {
			return false, fmt.Errorf("invalid Content-Length header: %s", lengthHeader)
		}
		log.Printf("Received Content-Length header: %d\n", *length)
	} else {
		log.Printf("No Content-Length header present. Assuming unknown length.\n")
	}

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		log.Printf("Rceived Range header: %s\n", rangeHeader)

		// Validate the Range header format
		if len(rangeHeader) < 6 || rangeHeader[:6] != "bytes=" {
			return false, fmt.Errorf("invalid Range header format: %s", rangeHeader)
		}

		// Handle cases like "bytes=start-end", "bytes=start-", and "bytes=-end"
		var start int64
		n, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, end)
		if err != nil {
			// Handle cases like "bytes=start-"
			n, err = fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			if err == nil && n == 1 {
				*bytesSent = start
				*end = -1 // End is unspecified
				log.Printf("Parsed Range header: start=%d, end=unspecified\n", start)
			} else {
				// Handle cases like "bytes=-end"
				n, err = fmt.Sscanf(rangeHeader, "bytes=-%d", end)
				if err != nil || n != 1 {
					return false, fmt.Errorf("invalid Range header format: %s", rangeHeader)
				}
				start = 0
				*bytesSent = 0 // Start from the beginning since no start value is specified
				log.Printf("Parsed Range header: start=unspecified, end=%d\n", *end)
			}
		} else {
			log.Printf("Parsed Range header: start=%d, end=%d\n", start, *end)
			if *end < start {
				return false, fmt.Errorf("invalid Range header: end (%d) is less than start (%d)", *end, start)
			}

			*bytesSent = start
		}

		if *length < 0 && *end >= 0 {
			*length = *end - start // Set length to end + 1 if end is specified
		}

		// Perform a HEAD request to check range support
		checkResp, err := fetchWithRetry(upstream, "HEAD", r.URL.Path, 1, rangeHeader)
		if err != nil {
			logUpstream("unable to check range support: %v\n", err)
			tempRangeHeader := fmt.Sprintf("bytes=%d-%d", *bytesSent, *bytesSent+1024)
			logUpstream("check range support with GET Request: %s\n", tempRangeHeader)
			checkResp, err = fetchWithRetry(upstream, "GET", r.URL.Path, 1, tempRangeHeader)
			if err != nil {
				return false, fmt.Errorf("unable to check range support: %v", err)
			}
		}
		defer checkResp.Body.Close()

		if checkResp.StatusCode == http.StatusOK || checkResp.StatusCode == http.StatusPartialContent {
			acceptRanges := checkResp.Header.Get("Accept-Ranges")
			contentRange := checkResp.Header.Get("Content-Range")
			if acceptRanges == "bytes" || contentRange != "" {
				logUpstream("Upstream server supports range requests.")
				*savedETag = checkResp.Header.Get("ETag")
				*savedLastModified = checkResp.Header.Get("Last-Modified")
				return TRUE_OR_SIMULATED_FALSE, nil
			} else {
				logUpstream("Upstream server does not support range requests.")
				return false, nil
			}
		}
		return false, fmt.Errorf("upstream server returned status: %d", checkResp.StatusCode)
	}

	// No Range header present
	log.Printf("No Range header present. Assuming full content request.")
	return false, nil
}

func resilientGet(r *http.Request, upstream string, hj http.Hijacker, w http.ResponseWriter) error {
	var bytesSent int64 = 0
	var start int64 = -1
	var end int64 = -1
	var length int64 = -1
	var savedETag, savedLastModified string
	var lastUpstreamError error

	// Check the client's Range request
	rangesPossible, err := checkClientRangeRequest(r, &start, &end, &length, &savedETag, &savedLastModified, upstream)
	if err != nil {
		rangesPossible = false
		lastUpstreamError = err
	}

	// Retry logic for streaming errors
	for attempt := 1; attempt <= maxRetries; attempt++ {
		rangeHeader := ""
		if rangesPossible && bytesSent > 0 {
			if end > start {
				rangeHeader = fmt.Sprintf("bytes=%d-%d", bytesSent, end) // Request remaining bytes
			} else {
				rangeHeader = fmt.Sprintf("bytes=%d-", bytesSent) // Request remaining bytes
			}
			log.Printf("Requesting range: %s\n", rangeHeader)
		} else if rangesPossible && bytesSent == 0 {
			rangeHeader = r.Header.Get("Range") // Request the initial range
			log.Printf("Requesting range: %s\n", rangeHeader)
		} else {
			log.Printf("No range requested. Sending full content.\n")
		}

		resp, err := fetchWithRetry(upstream, "GET", r.URL.Path, maxRetries, rangeHeader) // Pass the requested path and range
		if err != nil {
			lastUpstreamError = err
			log.Printf("Error fetching from upstream (attempt %d): %v\n", attempt, err)
			if attempt < maxRetries {
				log.Printf("Retrying fetch... (%d/%d)\n", attempt, maxRetries)
				sleepTime := min(60, (attempt)*(attempt))
				time.Sleep(retryDelay * time.Duration(sleepTime))
				continue
			}
			break
		}
		defer resp.Body.Close()

		// Validate Accept-Ranges header on the first successful response
		if attempt == 1 {
			acceptRanges := resp.Header.Get("Accept-Ranges")
			contentRange := resp.Header.Get("Content-Range")
			if acceptRanges == "bytes" || contentRange != "" {
				// FIXME
				rangesPossible = TRUE_OR_SIMULATED_FALSE
				logUpstream("Upstream server supports range requests.")
			} else {
				rangesPossible = false
				logUpstream("Upstream server does not support range requests.")
			}
			if start > 0 {
				bytesSent = start
			}
		}

		// Validate ETag and Last-Modified headers
		currentETag := resp.Header.Get("ETag")
		currentLastModified := resp.Header.Get("Last-Modified")
		if savedETag != "" && savedLastModified != "" {
			if currentETag != savedETag || currentLastModified != savedLastModified {
				logUpstream("Content changed during retries. ETag or Last-Modified mismatch.")
				conn, _, err := hj.Hijack()
				if err != nil {
					return err
				}

				// Close the hijacked raw tcp connection.
				_ = conn.Close()

				return fmt.Errorf("content changed during retries: ETag or Last-Modified mismatch")
			}
		} else {
			// Save ETag and Last-Modified headers on the first successful response
			savedETag = currentETag
			savedLastModified = currentLastModified
		}

		// Validate Content-Range header
		contentRange := resp.Header.Get("Content-Range")
		if rangesPossible && contentRange != "" {
			var rangeStart, rangeEnd, totalSize int64
			_, err := fmt.Sscanf(contentRange, "bytes %d-%d/%d", &rangeStart, &rangeEnd, &totalSize)
			if err != nil || rangeStart != bytesSent {
				logUpstream("Invalid or mismatched Content-Range: %s. Expected start: %d", contentRange, bytesSent)
				// Consume the necessary bytes to align with the expected range
				toConsume := bytesSent - rangeStart
				if toConsume > 0 {
					consumed := int64(0)
					// Consume the bytes in chunks of 1MB
					for consumed < toConsume {
						nextChunk := min(toConsume-consumed, 32*1024*1024)
						_, err := io.CopyN(io.Discard, resp.Body, nextChunk)
						if err != nil {
							logUpstream("Error consuming bytes to align with expected range: %v", err)
							break
						}
						consumed += nextChunk
						progress := 100 * consumed / toConsume // Correct progress calculation
						logUpstream("Consumed %d bytes of %d to align with expected range. Progress: %d%%\n", consumed, toConsume, progress)
					}
				}
			}
		} else if bytesSent > 0 {
			// If Content-Range is missing or ranges are not possible, assume the response starts from the beginning
			log.Printf("Content-Range header missing or ranges not supported. Consuming %d bytes to align.", bytesSent)
			consumed := int64(0)
			toConsume := bytesSent
			// consume the bytes in chunks of 1MB
			for consumed < toConsume {
				nextChunk := min(toConsume-consumed, 32*1024*1024)
				_, err := io.CopyN(io.Discard, resp.Body, nextChunk)
				if err != nil {
					logUpstream("Error consuming bytes to align with expected range: %v", err)
					break
				}
				consumed += nextChunk
				progress := 100 * consumed / toConsume // Correct progress calculation
				logUpstream("Consumed %d bytes of %d to align with expected range. Progress: %d%%\n", consumed, toConsume, progress)
			}
		}

		// Copy headers from upstream response (only on the first attempt)
		if bytesSent == start || (bytesSent == 0 && start == -1) {
			for key, values := range resp.Header {
				for _, value := range values {
					log.Printf("Header: %s: %s\n", key, value)
					w.Header().Add(key, value)
				}
			}
			if (start != 0 || end != 0) && !rangesPossible {
				if start == -1 {
					start = 0
				}
				if end == -1 {
					end = start + length
				}
				// TODO set Header correct in all cases.
				// Case 1 : Range header is requested by client and upstream server supports it
				// Case 2 : Range header is requested by client and upstream server does not support it
				// Case 3 : Range header is not requested by client
				// Case 4 : Range header is not requested by client and upstream server supports it (NOT NEEDED: Useless case)

				w.Header().Add("Accept-Ranges", "bytes")
				w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
				w.Header().Add("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, end+1))
			}
			log.Printf("Setting headers for the first response.\n")
			// Print header line by line
			for key, values := range w.Header() {
				log.Printf("< %s: %s\n", key, values)
			}
			w.WriteHeader(resp.StatusCode)
		}

		// Stream the response body to the client using a buffer
		buffer := make([]byte, 1024*1024) // 1MB buffer
		for {
			n, readErr := resp.Body.Read(buffer)
			if n > 0 {
				attempt = max(0, attempt-1)
				// Only write to the client if the block was read successfully
				if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
					return fmt.Errorf("Error writing to client (attempt %d): %v\n", attempt, writeErr)
				}
				bytesSent += int64(n) // Track how many bytes have been sent
				// log.Printf("Sent %d bytes to client. Total sent: %d bytes\n", n, bytesSent)
			}
			if readErr != nil {
				if readErr == io.EOF {
					// Successfully finished streaming
					log.Printf("Finished streaming data to client.\n")
					// log sent bytes
					log.Printf("Total bytes sent: %d\n", bytesSent-start)
					return nil
				}
				lastUpstreamError = readErr
				logUpstream("Error reading from upstream (attempt %d): %v\n", attempt, readErr)
				break
			}
		}

		// Retry if an error occurred
		if attempt < maxRetries {
			attempt++
			sleepTime := min(60, (attempt)*(attempt))
			logUpstream("Retrying in %d seconds\n", sleepTime)
			time.Sleep(retryDelay * time.Duration(sleepTime))
			logUpstream("Retrying streaming... (%d/%d)\n", attempt, maxRetries)
			continue
		}
	}

	// If all retries fail, send the last upstream error to the client
	if lastUpstreamError != nil {
		http.Error(w, fmt.Sprintf("Bad Gateway: %v", lastUpstreamError), http.StatusBadGateway)
	} else {
		http.Error(w, "Bad Gateway: Unable to stream data from upstream server.", http.StatusBadGateway)
	}
	return nil
}
