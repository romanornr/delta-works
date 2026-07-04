package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
)

// NewHTTPClient returns an HTTP client and base URL for a control-plane
// address in the same forms Listen accepts: "unix:///path" or host:port.
// The base URL's host is a placeholder for unix sockets; the dialer ignores
// it and connects to the socket.
func NewHTTPClient(addr string) (*http.Client, string) {
	path, ok := strings.CutPrefix(addr, "unix://")
	if !ok {
		// A bare ":port" listener address would leave the URL host empty.
		if strings.HasPrefix(addr, ":") {
			addr = "localhost" + addr
		}
		return http.DefaultClient, "http://" + addr
	}
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			if path == "" {
				return nil, errors.New("empty unix socket path")
			}
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		},
	}}, "http://localhost"
}
