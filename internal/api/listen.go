package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strings"
)

// Listen opens the control-plane listener. A "unix://" address listens on a
// Unix socket restricted to the owning user: file permissions are the whole
// auth story until a TCP deployment adds token auth (ADR-0007). Anything
// else is a TCP host:port.
func Listen(ctx context.Context, addr string) (net.Listener, error) {
	path, ok := strings.CutPrefix(addr, "unix://")
	if !ok {
		return new(net.ListenConfig).Listen(ctx, "tcp", addr)
	}
	if path == "" {
		return nil, errors.New("empty unix socket path")
	}
	if info, err := os.Stat(path); err == nil {
		if info.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("refusing to replace non-socket file %s", path)
		}
		if conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path); err == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("socket %s is in use by a live process", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket %s: %w", path, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat socket path %s: %w", path, err)
	}
	ln, err := new(net.ListenConfig).Listen(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("restrict socket %s: %w", path, err)
	}
	return ln, nil
}
