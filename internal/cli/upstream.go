package cli

// upstream.go — detect a running upstream z13ctl daemon, which would fight
// z13ctl-plus over the same hardware.

import (
	"net"
	"os"
	"time"
)

// UpstreamSocketPath is where an upstream (dahui/z13ctl) daemon listens.
func UpstreamSocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = "/tmp"
	}
	return dir + "/z13ctl/z13ctl.sock"
}

// UpstreamDaemonRunning reports whether an upstream daemon accepts connections.
func UpstreamDaemonRunning() bool {
	conn, err := net.DialTimeout("unix", UpstreamSocketPath(), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
