package embed

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/microyahoo/etcd-test/etcdserver"
	"github.com/microyahoo/etcd-test/etcdserver/etcdhttp"
	"github.com/microyahoo/etcd-test/pkg/types"
	"github.com/microyahoo/etcd-test/rafthttp"
)

// Etcd contains a running etcd server and its listeners.
type Etcd struct {
	Peers   []*peerListener
	Clients []net.Listener
	// a map of contexts for the servers that serves client requests.
	// sctxs            map[string]*serveCtx
	metricsListeners []net.Listener

	Server *etcdserver.EtcdServer

	cfg   Config
	stopc chan struct{}
	errc  chan error

	closeOnce sync.Once
}

type peerListener struct {
	net.Listener
	serve func() error
	close func(context.Context) error
}

// configure peer handlers after rafthttp.Transport started
func (e *Etcd) servePeers() (err error) {
	etcdhttp.NewPeerHandler(e.Server)
	return nil
	// ph := etcdhttp.NewPeerHandler(e.Server)
}

func configurePeerListeners(cfg *Config) (peers []*peerListener, err error) {
	peers = make([]*peerListener, len(cfg.LPUrls))
	defer func() {
		if err == nil {
			return
		}
		for i := range peers {
			if peers[i] != nil && peers[i].close != nil {
				log.Printf("closing peer listener, address: %#v, err: %v", cfg.LPUrls[i].String(), err)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				peers[i].close(ctx)
				cancel()
			}
		}
	}()

	for i, u := range cfg.LPUrls {
		if u.Scheme == "http" {
			// if !cfg.PeerTLSInfo.Empty() {
			// 	cfg.logger.Warn("scheme is HTTP while key and cert files are present; ignoring key and cert files", zap.String("peer-url", u.String()))
			// }
			// if cfg.PeerTLSInfo.ClientCertAuth {
			// 	cfg.logger.Warn("scheme is HTTP while --peer-client-cert-auth is enabled; ignoring client cert auth for this URL", zap.String("peer-url", u.String()))
			// }
		}
		peers[i] = &peerListener{close: func(context.Context) error { return nil }}
		peers[i].Listener, err = rafthttp.NewListener(u)
		if err != nil {
			return nil, err
		}
		// once serve, overwrite with 'http.Server.Shutdown'
		peers[i].close = func(context.Context) error {
			return peers[i].Listener.Close()
		}
	}
	return peers, nil
}

// StartEtcd launches the etcd server and HTTP handlers for client/server communication.
// The returned Etcd.Server is not guaranteed to have joined the cluster. Wait
// on the Etcd.Server.ReadyNotify() channel to know when it completes and is ready for use.
func StartEtcd(inCfg *Config) (e *Etcd, err error) {
	e = &Etcd{cfg: *inCfg, stopc: make(chan struct{})}

	cfg := &e.cfg
	if e.Peers, err = configurePeerListeners(cfg); err != nil {
		return e, err
	}

	urlsmap, err := types.NewURLsMap(inCfg.InitialCluster)
	if err != nil {
		return e, err
	}
	srvcfg := etcdserver.ServerConfig{
		Name:                inCfg.Name,
		InitialPeerURLsMap:  urlsmap,
		InitialClusterToken: inCfg.InitialClusterToken,
		ClientURLs:          inCfg.ACUrls,
		PeerURLs:            inCfg.APUrls,
		DataDir:             inCfg.Dir,
	}
	if e.Server, err = etcdserver.NewServer(srvcfg); err != nil {
		return e, err
	}
	e.Server.Start()

	if err = e.servePeers(); err != nil {
		return e, err
	}
	return e, nil
}
