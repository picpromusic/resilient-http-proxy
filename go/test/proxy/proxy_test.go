package test_proxy

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
	"tkse-proxies/test"
)

func TestProxyFunctionality(t *testing.T) {
	setupProxyTest(t)

	t.Log("Fetching data...")
	test.FetchCompleteFile(t, test.BaseURLProxy)

	t.Log("Splitting file into blocks...")
	test.SplitFileIntoBlocks(t)

	sha1Complete := test.CalculateSHA1(t, test.CompleteFile)
	t.Logf("SHA1 of complete file: %s", sha1Complete)

	t.Log("Calculating SHA1 of concatenated blocks...")
	sha1Blocks, err := test.CalculateSHA1OfBlocks(test.DataDir, test.CompleteSize, test.BlockSize)
	if err != nil {
		t.Fatalf("Failed to calculate SHA1 of concatenated blocks: %v", err)
	}

	if sha1Complete != sha1Blocks {
		t.Fatalf("SHA1 mismatch: complete=%s, blocks=%s", sha1Complete, sha1Blocks)
	}

	t.Log("Verifying range requests...")
	test.VerifyRangeRequests(t, test.BaseURLProxy, test.DataDir, test.CompleteSize, test.BlockSize)

	t.Log("All tests passed successfully.")
}

func TestProxyHandlesDisconnectionByTcpKill(t *testing.T) {
	// Setup and start the backend and proxy
	setupProxyTest(t,
		test.WithBackendWaitEveryNElements(test.CompleteSize/10),
		test.WithBackendLogFile("/tmp/backend.log"),
		test.WithProxyLogFile("/tmp/proxy.log"))

	done := make(chan error, 1)
	go func() {
		err := test.FetchData(test.BaseURLProxy, test.CompleteSize, test.CompleteFile)
		done <- err
	}()

	// Wait for a few seconds to simulate the disconnection
	time.Sleep(2 * time.Second)

	// Simulate a network disconnection using tcpkill
	t.Log("Simulating network disconnection with tcpkill...")
	tcpkillCmd := exec.Command("sudo", "tcpkill", "-i", "lo", "port", fmt.Sprintf("%d", test.BackendPort))
	err := tcpkillCmd.Start()
	if err != nil {
		t.Fatalf("Failed to start tcpkill: %v", err)
	}
	t.Cleanup(func() {
		if tcpkillCmd != nil && tcpkillCmd.Process != nil {
			tcpkillCmd.Process.Kill()
		}
	})
	// Wait for a few seconds to simulate the disconnection
	time.Sleep(5 * time.Second)

	// Stop tcpkill to restore the connection
	t.Log("Stopping tcpkill to restore network connection...")
	err = tcpkillCmd.Process.Kill()
	if err != nil {
		t.Fatalf("Failed to stop tcpkill: %v", err)
	}

	// Wait for the download to complete
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Download failed after network disconnection: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Download timed out after network disconnection")
	}

	// Verify the downloaded file
	t.Log("Verifying downloaded file...")
	sha1Complete := test.CalculateSHA1(t, test.CompleteFile)

	test.FetchCompleteFile(t, test.BaseURLProxy)
	sha1CompleteUninterrupted := test.CalculateSHA1(t, test.CompleteFile)

	t.Logf("SHA1 of complete file: %s", sha1Complete)
	t.Logf("SHA1 of complete file: %s", sha1CompleteUninterrupted)

	// Assert that the SHA1 hashes match
	if sha1Complete != sha1CompleteUninterrupted {
		t.Fatalf("SHA1 mismatch: complete=%s, uninterrupted=%s", sha1Complete, sha1CompleteUninterrupted)
	}
}

func TestProxyHandlesBackendCrash(t *testing.T) {
	test.CreateDataDir(t)

	cmdBackend := test.StartBackendService(t,
		test.WithBackendLogFile("/tmp/backend.log"),
		test.WithBackendWaitEveryNElements(test.CompleteSize/10))

	test.StartProxyService(t, test.WithProxyLogFile("/tmp/proxy.log"))

	done := make(chan error, 1)
	go func() {
		err := test.FetchData(test.BaseURLProxy, test.CompleteSize, test.CompleteFile)
		done <- err
	}()

	// Wait for a few seconds to simulate the disconnection
	time.Sleep(2 * time.Second)

	cmdBackend.Process.Kill()

	// Wait for a few seconds to simulate the disconnection
	time.Sleep(5 * time.Second)

	test.StartBackendService(t, test.WithBackendLogFile("/tmp/backend.log"))

	// Wait for the download to complete
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Download failed after network disconnection: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Download timed out after network disconnection")
	}

	// Verify the downloaded file
	t.Log("Verifying downloaded file...")
	sha1Complete := test.CalculateSHA1(t, test.CompleteFile)

	test.FetchCompleteFile(t, test.BaseURLProxy)
	sha1CompleteUninterrupted := test.CalculateSHA1(t, test.CompleteFile)

	t.Logf("SHA1 of complete file: %s", sha1Complete)
	t.Logf("SHA1 of complete file: %s", sha1CompleteUninterrupted)

	// Assert that the SHA1 hashes match
	if sha1Complete != sha1CompleteUninterrupted {
		t.Fatalf("SHA1 mismatch: complete=%s, uninterrupted=%s", sha1Complete, sha1CompleteUninterrupted)
	}
}

func TestProxyHandlesRetriesIfBackendserverStartedToLate(t *testing.T) {
	// Setup and start the backend and proxy
	cmdProxy := test.StartProxyService(t, test.WithProxyLogFile("/tmp/proxy.log"))
	t.Cleanup(func() {
		if cmdProxy != nil && cmdProxy.Process != nil {
			cmdProxy.Process.Kill()
		}
	})

	done := make(chan error, 1)
	go func() {
		err := test.FetchData(test.BaseURLProxy, test.CompleteSize, test.CompleteFile)
		done <- err
	}()

	// Wait for a few seconds to simulate the disconnection
	time.Sleep(10 * time.Second)

	test.StartBackendService(t, test.WithBackendLogFile("/tmp/backend.log"))

	// Wait for the download to complete
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Download failed after network disconnection: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Download timed out after network disconnection")
	}

	// Verify the downloaded file
	t.Log("Verifying downloaded file...")
	sha1Complete := test.CalculateSHA1(t, test.CompleteFile)

	test.FetchCompleteFile(t, test.BaseURLProxy)
	sha1CompleteUninterrupted := test.CalculateSHA1(t, test.CompleteFile)

	t.Logf("SHA1 of complete file: %s", sha1Complete)
	t.Logf("SHA1 of complete file: %s", sha1CompleteUninterrupted)

	// Assert that the SHA1 hashes match
	if sha1Complete != sha1CompleteUninterrupted {
		t.Fatalf("SHA1 mismatch: complete=%s, uninterrupted=%s", sha1Complete, sha1CompleteUninterrupted)
	}
}

func setupProxyTest(t *testing.T, options ...interface{}) {
	t.Helper()
	test.CreateDataDir(t)

	var backendOpts []test.BackendOption
	var proxyOpts []test.ProxyOption

	for _, opt := range options {
		switch v := opt.(type) {
		case test.BackendOption:
			backendOpts = append(backendOpts, v)
		case test.ProxyOption:
			proxyOpts = append(proxyOpts, v)
		default:
			t.Fatalf("Unknown option type: %T", v)
		}
	}

	test.StartBackendService(t, backendOpts...)
	test.StartProxyService(t, proxyOpts...)
}
