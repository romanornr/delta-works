package api

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListen(t *testing.T) {
	t.Parallel()

	t.Run("empty unix path rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := Listen(t.Context(), "unix://"); err == nil {
			t.Fatal("want error for empty socket path")
		}
	})

	t.Run("regular file at socket path refused", func(t *testing.T) {
		t.Parallel()
		// Sockets live in os.TempDir, not t.TempDir: unix socket paths are
		// limited to ~108 bytes and test names push past that.
		path := filepath.Join(os.TempDir(), "api-listen-refuse.sock")
		t.Cleanup(func() { _ = os.Remove(path) })
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Listen(t.Context(), "unix://"+path); err == nil {
			t.Fatal("want error when the path holds a regular file")
		}
	})

	t.Run("live socket refused", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(os.TempDir(), "api-listen-live.sock")
		t.Cleanup(func() { _ = os.Remove(path) })
		ln, err := Listen(t.Context(), "unix://"+path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		if _, err := Listen(t.Context(), "unix://"+path); err == nil {
			t.Fatal("want error when another process is listening")
		}
	})

	t.Run("unix socket restricted and stale socket replaced", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(os.TempDir(), "api-listen-test.sock")
		t.Cleanup(func() { _ = os.Remove(path) })
		stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		stale.SetUnlinkOnClose(false)
		if err := stale.Close(); err != nil {
			t.Fatal(err)
		}
		ln, err := Listen(t.Context(), "unix://"+path)
		if err != nil {
			t.Fatalf("listen with stale socket present: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("got socket perms %o, want 600", perm)
		}
		if err := ln.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("socket file should be unlinked on close")
		}
	})
}
