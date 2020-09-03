package embed

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/microyahoo/etcd-test/etcdserver"
	"github.com/microyahoo/etcd-test/etcdserver/etcdhttp"
	"github.com/microyahoo/etcd-test/etcdserver/v3rpc"
	"github.com/microyahoo/etcd-test/pkg/types"
	"github.com/microyahoo/etcd-test/rafthttp"
)

// Etcd contains a running etcd server and its listeners.
type Etcd struct {
	Peers []*peerListener

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
	ph := etcdhttp.NewPeerHandler(e.Server)

	for _, p := range e.Peers {
		// u := p.Listener.Addr().String()
		gs := v3rpc.Server(e.Server)
		m := cmux.New(p.Listener)
		go gs.Serve(m.Match(cmux.HTTP2()))
		srv := &http.Server{
			Handler:     grpcHandlerFunc(gs, ph),
			ReadTimeout: 5 * time.Minute,
			// ErrorLog:    defaultLog.New(ioutil.Discard, "", 0), // do not log user error
		}
		go srv.Serve(m.Match(cmux.Any()))
		p.serve = func() error { return m.Serve() }

		// p.close = func(ctx context.Context) error {
		// 	// gracefully shutdown http.Server
		// 	// close open listeners, idle connections
		// 	// until context cancel or time-out
		// 	e.cfg.logger.Info(
		// 		"stopping serving peer traffic",
		// 		zap.String("address", u),
		// 	)
		// 	stopServers(ctx, &servers{secure: peerTLScfg != nil, grpc: gs, http: srv})
		// 	e.cfg.logger.Info(
		// 		"stopped serving peer traffic",
		// 		zap.String("address", u),
		// 	)
		// 	return nil
		// }

	}

	// start peer servers in a goroutine
	for _, pl := range e.Peers {
		go func(l *peerListener) {
			u := l.Addr().String()
			e.cfg.logger.Info(
				"serving peer traffic",
				zap.String("address", u),
			)
			err := l.serve()
			e.cfg.logger.Warn("Failed to serve", zap.String("address", u), zap.Error(err))
		}(pl)
	}

	return nil
}

func configurePeerListeners(cfg *Config) (peers []*peerListener, err error) {
	peers = make([]*peerListener, len(cfg.LPUrls))
	defer func() {
		if err == nil {
			return
		}
		for i := range peers {
			if peers[i] != nil && peers[i].close != nil {
				cfg.logger.Warn("closing peer listener",
					zap.String("address", cfg.LPUrls[i].String()),
					zap.Error(err),
				)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				peers[i].close(ctx)
				cancel()
			}
		}
	}()

	for i, u := range cfg.LPUrls {
		peers[i] = &peerListener{close: func(context.Context) error { return nil }}
		peers[i].Listener, err = rafthttp.NewListener(u)
		if err != nil {
			return nil, err
		}
		// once serve, overwrite with 'http.Server.Shutdown'
		peers[i].close = func(context.Context) error {
			u := peers[i].Listener.Addr().String()
			cfg.logger.Info(
				"stopping serving peer traffic",
				zap.String("address", u),
			)
			defer func() {
				cfg.logger.Info("stopped serving peer traffic", zap.String("address", u))
			}()
			return peers[i].Listener.Close()
		}
	}
	return peers, nil
}

// StartEtcd launches the etcd server and HTTP handlers for client/server communication.
// The returned Etcd.Server is not guaranteed to have joined the cluster. Wait
// on the Etcd.Server.ReadyNotify() channel to know when it completes and is ready for use.
func StartEtcd(inCfg *Config) (e *Etcd, err error) {
	if err = inCfg.Validate(); err != nil {
		return nil, err
	}

	e = &Etcd{cfg: *inCfg, stopc: make(chan struct{})}

	e.cfg.logger.Info("configuring peer listeners",
		zap.Strings("listen-peer-urls", e.cfg.getLPURLs()),
	)
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
		Logger:              inCfg.logger,
		LoggerConfig:        inCfg.loggerConfig,
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

// MarshalLogObject implements zapcore.ObjectMarshaller interface.
func (e *Etcd) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if e == nil {
		return nil
	}

	if err := enc.AddObject("Config", &e.cfg); err != nil {
		fmt.Println("1", err)
		return err
	}
	if err := enc.AddObject("Server", e.Server); err != nil {
		fmt.Println("2", err)
		return err
	}
	if err := enc.AddReflected("Peers", e.Peers); err != nil {
		fmt.Println("3", err)
		return err
	}
	return nil
}
