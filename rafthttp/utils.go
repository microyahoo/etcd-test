package rafthttp

import (
	"net"
	"net/url"

	"github.com/microyahoo/etcd-test/pkg/transport"
)

// NewListener returns a listener for raft message transfer between peers.
// It uses timeout listener to identify broken streams promptly.
func NewListener(u url.URL) (net.Listener, error) {
	return transport.NewTimeoutListener(u.Host, u.Scheme, ConnReadTimeout, ConnWriteTimeout)
}
