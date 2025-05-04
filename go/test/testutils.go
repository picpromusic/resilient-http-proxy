package test

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func CalculateSHA1(t *testing.T, filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	hasher := sha1.New()
	_, err = io.Copy(hasher, file)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func CalculateSHA1OfBlocks(dir string, totalSize, blockSize int) (string, error) {
	hash := sha1.New()

	for i := 0; i < totalSize; i += blockSize {
		blockFile := filepath.Join(dir, fmt.Sprintf("block_%d", i))
		file, err := os.Open(blockFile)
		if err != nil {
			return "", err
		}

		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func VerifyRangeRequests(t *testing.T, baseURL string, dir string, totalSize, blockSize int) {
	for i := 0; i < totalSize; i += blockSize {

		blockFile := filepath.Join(dir, fmt.Sprintf("block_%d", i))
		storedSHA1 := CalculateSHA1(t, blockFile)
		fetchedSHA1 := CalculateSHA1(t, DownloadRange(t, baseURL, i))

		if storedSHA1 != fetchedSHA1 {
			t.Fatalf("SHA1 mismatch for block %d: stored=%s, fetched=%s", i, storedSHA1, fetchedSHA1)
		} else {
			fmt.Printf("SHA1 verified for block %d\n", i)
		}
	}
}

func StartProxyService(t *testing.T, opts ...ProxyOption) *exec.Cmd {
	t.Helper()

	// Default configuration
	cfg := &ProxyConfig{
		Port:     3000,
		Upstream: "http://127.0.0.1:5000",
		LogFile:  "/tmp/proxy.log",
		Args:     []string{},
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	var logFile *os.File
	var err error

	if cfg.LogFile != "" {
		logFile, err = os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			t.Fatalf("Failed to open log file: %v", err)
		}
		defer logFile.Close()
	}

	// Start the proxy service
	t.Log("Starting proxy server...")
	args := append([]string{"-port", strconv.Itoa(cfg.Port), "-upstream", cfg.Upstream}, cfg.Args...)
	cmd := exec.Command("resilientproxy", args...)
	if cfg.LogFile != "" {
		// Pipe stdout and stderr to the log file
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start proxy server: %v", err)
	}
	t.Cleanup(func() {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
	})

	// Wait for the backend server to be ready
	err = waitForServerReady(cfg.Port, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to start backend server: %v", err)
		cmd.Process.Kill()
	}

	return cmd
}

func StartBackendService(t *testing.T, opts ...BackendOption) *exec.Cmd {
	t.Helper()

	// Default configuration
	cfg := &BackendConfig{
		Port:               5000,
		Args:               []string{},
		LogFile:            "",
		WaitEveryNElements: 0,
		CurrentModified:    false, // Default value
		RandomEtag:         false, // Default value
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	var logFile *os.File
	var err error

	if cfg.LogFile != "" {
		logFile, err = os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			t.Fatalf("Failed to open log file: %v", err)
		}
		defer logFile.Close()
	}

	// Start the backend service
	fmt.Println("Starting the backend server...")
	args := []string{
		"-port", strconv.Itoa(cfg.Port),
		"-waitEveryNElements", strconv.Itoa(cfg.WaitEveryNElements),
	}
	if cfg.CurrentModified {
		args = append(args, "-currentModified")
	}
	if cfg.RandomEtag {
		args = append(args, "-randomEtag")
	}
	args = append(args, cfg.Args...)
	cmd := exec.Command("randombackend", args...)
	if cfg.LogFile != "" {
		// Pipe stdout and stderr to the log file
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start backend server: %v", err)
	}
	t.Cleanup(func() {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
	})

	// Wait for the backend server to be ready
	err = waitForServerReady(cfg.Port, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to start backend server: %v", err)
		cmd.Process.Kill()
	}

	return cmd
}

func waitForServerReady(port int, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server on port %d not ready after %v", port, timeout)
}

func CreateDataDir(t *testing.T) {
	err := os.MkdirAll(DataDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create data directory: %v", err)
	}

	files, err := os.ReadDir(DataDir)
	if err != nil {
		t.Fatalf("Failed to read data directory: %v", err)
	}
	for _, file := range files {
		os.RemoveAll(filepath.Join(DataDir, file.Name()))
	}
}

func DownloadRange(t *testing.T, baseURL string, i int) string {
	start := i
	end := i + BlockSize - 1
	rangeFile := filepath.Join(DataDir, fmt.Sprintf("range_%d", i))

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/generate/%d", baseURL, CompleteSize), nil)
	if err != nil {
		t.Fatalf("Failed to create range request: %v", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to fetch range %d-%d: %v", start, end, err)
	}
	defer resp.Body.Close()

	outFile, err := os.Create(rangeFile)
	if err != nil {
		t.Fatalf("Failed to create range file: %v", err)
	}
	_, err = io.Copy(outFile, resp.Body)
	outFile.Close()
	if err != nil {
		t.Fatalf("Failed to save range file: %v", err)
	}
	return rangeFile
}

func SplitFileIntoBlocks(t *testing.T) {
	fmt.Println("Splitting file into blocks...")
	cmdSplit := exec.Command("split", "-b", strconv.Itoa(BlockSize), CompleteFile, filepath.Join(DataDir, "block_"))
	err := cmdSplit.Run()
	if err != nil {
		t.Fatalf("Failed to split file: %v", err)
	}

	// Verify the number of blocks
	files, err := os.ReadDir(DataDir)
	if err != nil {
		t.Fatalf("Failed to read data directory: %v", err)
	}
	blockFiles := []string{}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "block_") {
			blockFiles = append(blockFiles, file.Name())
		}
	}
	if len(blockFiles) != Blocks {
		t.Fatalf("Expected %d blocks, but found %d", Blocks, len(blockFiles))
	}

	// Rename blocks
	fmt.Println("Renaming blocks...")
	sort.Strings(blockFiles)
	for i, blockFile := range blockFiles {
		newName := filepath.Join(DataDir, fmt.Sprintf("block_%d", i*BlockSize))
		err := os.Rename(filepath.Join(DataDir, blockFile), newName)
		if err != nil {
			t.Fatalf("Failed to rename block: %v", err)
		}
	}
}

func FetchCompleteFile(t *testing.T, baseUrl string) {
	fmt.Println("Fetching 100000 bytes...")
	resp, err := http.Get(fmt.Sprintf("%s/generate/%d", baseUrl, CompleteSize))
	if err != nil {
		t.Fatalf("Failed to fetch data: %v", err)
	}
	defer resp.Body.Close()

	outFile, err := os.Create(CompleteFile)
	if err != nil {
		t.Fatalf("Failed to create complete file: %v", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		t.Fatalf("Failed to save complete file: %v", err)
	}
}

func FetchData(baseUrl string, size int, outputFile string) error {
	cmd := exec.Command("curl", fmt.Sprintf("%s/generate/%d", baseUrl, size), "-o", outputFile)
	return cmd.Run()
}
