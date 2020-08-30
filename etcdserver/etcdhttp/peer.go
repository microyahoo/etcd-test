package etcdhttp

import (
	"net/http"

	"github.com/microyahoo/etcd-test/etcdserver"
	"github.com/microyahoo/etcd-test/rafthttp"
)

// NewPeerHandler generates an http.Handler to handle etcd peer requests.
func NewPeerHandler(s etcdserver.ServerPeer) http.Handler {
	return newPeerHandler(s.RaftHandler())
}

func newPeerHandler(raftHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", http.NotFound)
	mux.Handle(rafthttp.RaftPrefix, raftHandler)
	mux.Handle(rafthttp.RaftPrefix+"/", raftHandler)
	return mux
}
