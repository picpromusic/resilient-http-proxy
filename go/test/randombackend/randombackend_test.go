package test_randombackend

import (
	"crypto/sha1"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"resilient-http-proxy/test"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T) *exec.Cmd {
	t.Helper()

	// Create the data directory
	test.CreateDataDir(t)

	// Start the backend server
	return test.StartBackendService(t, test.WithBackendPort(test.BackendPort), test.WithBackendLogFile("/tmp/backend.log"))
}

func setupRandomEtagCurrentModified(t *testing.T) *exec.Cmd {
	t.Helper()

	// Create the data directory
	test.CreateDataDir(t)

	// Start the backend server
	return test.StartBackendService(t, test.WithBackendPort(test.BackendPort), test.WithBackendLogFile("/tmp/backend.log"), test.WithBackendRandomEtag(true), test.WithBackendCurrentModified(true))
}

func teardown(cmd *exec.Cmd) {
	fmt.Println("Shutting down the backend server...")
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
}

func TestRandomBackend(t *testing.T) {
	// Setup and teardown
	cmd := setup(t)
	t.Cleanup(func() {
		teardown(cmd)
	})

	// Step 1: Fetch 100000 bytes and save to data/complete
	test.FetchCompleteFile(t, test.BaseURLBackend)

	// Step 2: Split the file into blocks
	test.SplitFileIntoBlocks(t)

	// Step 3: Calculate SHA1 of the complete file
	fmt.Println("Calculating SHA1 of complete file...")
	sha1Complete := test.CalculateSHA1(t, test.CompleteFile)
	fmt.Printf("SHA1 of complete file: %s\n", sha1Complete)

	// Step 4: Calculate SHA1 of concatenated blocks
	fmt.Println("Calculating SHA1 of concatenated blocks...")
	hasher := sha1.New()
	for i := 0; i < test.CompleteSize; i += test.BlockSize {
		blockFile := filepath.Join(test.DataDir, fmt.Sprintf("block_%d", i))
		file, err := os.Open(blockFile)
		if err != nil {
			t.Fatalf("Failed to open block file: %v", err)
		}
		_, err = io.Copy(hasher, file)
		file.Close()
		if err != nil {
			t.Fatalf("Failed to read block file: %v", err)
		}
	}
	sha1Blocks := fmt.Sprintf("%x", hasher.Sum(nil))
	fmt.Printf("SHA1 of concatenated blocks: %s\n", sha1Blocks)

	// Step 5: Compare SHA1 checksums
	if sha1Complete != sha1Blocks {
		t.Fatalf("SHA1 checksums do not match!")
	}
	fmt.Println("SHA1 checksums match!")

	// Step 6: Fetch each block using range requests and verify SHA1
	test.VerifyRangeRequests(t, test.BaseURLBackend, test.DataDir, test.CompleteSize, test.BlockSize)
	fmt.Println("All range requests verified successfully.")
}

func TestDefaultEtagAndModifiedSince(t *testing.T) {
	// Setup and teardown
	cmd := setup(t)
	t.Cleanup(func() {
		teardown(cmd)
	})

	randomSize := 100 + rand.Intn(900)

	resp, err := http.Get(fmt.Sprintf("%s/generate/%d", test.BaseURLBackend, randomSize))
	if err != nil {
		t.Fatalf("Failed to fetch data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", resp.StatusCode)
	}

	etag := resp.Header.Get("Etag")
	lastModified := resp.Header.Get("Last-Modified")

	if etag != fmt.Sprintf("%d", randomSize) {
		t.Errorf("Expected ETag to be %d, got %s", randomSize, etag)
	}

	if lastModified != "Thu, 01 Jan 1970 00:00:00 GMT" {
		t.Errorf("Expected Last-Modified to be Thu, 01 Jan 1970 00:00:00 GMT, got %s", lastModified)
	}

}

func TestRandomEtagAndCurrentModifiedSince(t *testing.T) {
	// Setup and teardown
	cmd := setupRandomEtagCurrentModified(t)
	t.Cleanup(func() {
		teardown(cmd)
	})

	randomSize := 100 + rand.Intn(900)

	resp, err := http.Get(fmt.Sprintf("%s/generate/%d", test.BaseURLBackend, randomSize))
	if err != nil {
		t.Fatalf("Failed to fetch data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", resp.StatusCode)
	}

	etag := resp.Header.Get("Etag")
	lastModified := resp.Header.Get("Last-Modified")

	if etag == fmt.Sprintf("%d", randomSize) {
		t.Errorf("Expected ETag to be %d, got %s", randomSize, etag)
	}

	currentDate := time.Now().Format("02 Jan 2006")
	if !strings.Contains(lastModified, currentDate) {
		t.Errorf("Expected Last-Modified to contain the current date %s, got %s", currentDate, lastModified)
	}

}
