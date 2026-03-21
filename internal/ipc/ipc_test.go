package ipc

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerClientLaunchRoundtrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")

	srv, err := NewServer(sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	var called atomic.Bool
	srv.SetHandler(Handler{
		OnLaunch: func(req LaunchRequest) LaunchResponse {
			called.Store(true)
			if req.ProjectDir != "/home/user/project" {
				t.Errorf("unexpected ProjectDir: %s", req.ProjectDir)
			}
			if req.ResumeID != "abc123" {
				t.Errorf("unexpected ResumeID: %s", req.ResumeID)
			}
			return LaunchResponse{OK: true, SessionID: "sess-001"}
		},
	})

	go srv.Serve()

	resp, err := Launch(sock, LaunchRequest{
		ProjectDir: "/home/user/project",
		ResumeID:   "abc123",
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if !called.Load() {
		t.Error("OnLaunch handler was not called")
	}
	if !resp.OK {
		t.Error("expected OK=true")
	}
	if resp.SessionID != "sess-001" {
		t.Errorf("expected SessionID=sess-001, got %s", resp.SessionID)
	}
}

func TestServerExitNotification(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")

	srv, err := NewServer(sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	var gotWindowID atomic.Value
	done := make(chan struct{})
	srv.SetHandler(Handler{
		OnExit: func(notif ExitNotification) {
			gotWindowID.Store(notif.WindowID)
			close(done)
		},
	})

	go srv.Serve()

	err = NotifyExit(sock, ExitNotification{WindowID: "@42"})
	if err != nil {
		t.Fatalf("NotifyExit: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnExit")
	}

	if id, ok := gotWindowID.Load().(string); !ok || id != "@42" {
		t.Errorf("expected WindowID=@42, got %v", gotWindowID.Load())
	}
}

func TestClientNoServer(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "nonexistent.sock")

	_, err := Launch(sock, LaunchRequest{ProjectDir: "/tmp"})
	if err == nil {
		t.Fatal("expected error when no server is running")
	}

	err = NotifyExit(sock, ExitNotification{WindowID: "@1"})
	if err == nil {
		t.Fatal("expected error when no server is running")
	}
}

func TestStaleSocketCleanup(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "stale.sock")

	// Create a stale socket file (just a regular file, no listener)
	if err := os.WriteFile(sock, []byte{}, 0o600); err != nil {
		t.Fatalf("create stale socket file: %v", err)
	}

	srv, err := NewServer(sock)
	if err != nil {
		t.Fatalf("NewServer with stale socket: %v", err)
	}
	defer srv.Close()

	// Verify server works by doing a launch roundtrip
	srv.SetHandler(Handler{
		OnLaunch: func(req LaunchRequest) LaunchResponse {
			return LaunchResponse{OK: true}
		},
	})

	go srv.Serve()

	resp, err := Launch(sock, LaunchRequest{ProjectDir: "/tmp"})
	if err != nil {
		t.Fatalf("Launch after stale cleanup: %v", err)
	}
	if !resp.OK {
		t.Error("expected OK=true after stale socket cleanup")
	}
}

func TestServerCloseRemovesSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")

	srv, err := NewServer(sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	go srv.Serve()

	// Socket file should exist
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("socket file should exist: %v", err)
	}

	srv.Close()

	// Socket file should be gone
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Error("socket file should not exist after Close()")
	}
}

func TestConcurrentLaunches(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")

	srv, err := NewServer(sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	var counter atomic.Int64
	srv.SetHandler(Handler{
		OnLaunch: func(req LaunchRequest) LaunchResponse {
			counter.Add(1)
			return LaunchResponse{OK: true, SessionID: req.ProjectDir}
		},
	})

	go srv.Serve()

	const numClients = 20
	var wg sync.WaitGroup
	errs := make(chan error, numClients)

	for i := range numClients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := Launch(sock, LaunchRequest{
				ProjectDir: filepath.Join("/tmp", "project", string(rune('A'+i))),
			})
			if err != nil {
				errs <- err
				return
			}
			if !resp.OK {
				errs <- fmt.Errorf("client %d: expected OK=true", i)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	if got := counter.Load(); got != numClients {
		t.Errorf("expected %d handler calls, got %d", numClients, got)
	}
}
