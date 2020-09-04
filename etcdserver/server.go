package etcdserver

import (
	"context"
	"net/http"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/microyahoo/etcd-test/pkg/types"
	"github.com/microyahoo/etcd-test/raft"
	pb "github.com/microyahoo/etcd-test/raft/raftpb"
	"github.com/microyahoo/etcd-test/rafthttp"
)

type ServerPeer interface {
	// ServerV2
	RaftHandler() http.Handler
	// LeaseHandler() http.Handler
}

type EtcdServer struct {
	r raftNode

	id types.ID

	cluster *RaftCluster

	Cfg ServerConfig

	lgMu *sync.RWMutex
	lg   *zap.Logger

	// peerRt used to send requests (version, lease) to peers.
	peerRt http.RoundTripper
}

var _ ServerPeer = (*EtcdServer)(nil)
var _ rafthttp.Raft = (*EtcdServer)(nil)

func (s *EtcdServer) Process(ctx context.Context, m pb.Message) error {
	return s.r.Step(ctx, m)
}

func (s *EtcdServer) Start() {
	s.lg.Info("Start to run etcdserver")
	s.start()
}

func (s *EtcdServer) start() {
	go s.run()
}

func (s *EtcdServer) run() {
	s.r.start()

	for {
		select {
		case ap := <-s.r.apply():
			s.lg.Info("Receive apply", zap.Any("apply", ap))
		}
	}
}

func (s *EtcdServer) RaftHandler() http.Handler { return s.r.transport.Handler() }

// ServerConfig holds the configuration of etcd as taken from the command line or discovery.
type ServerConfig struct {
	Name       string
	ClientURLs types.URLs
	PeerURLs   types.URLs
	DataDir    string

	InitialPeerURLsMap  types.URLsMap
	InitialClusterToken string

	// Logger logs server-side operations.
	// If not nil, it disables "capnslog" and uses the given logger.
	Logger *zap.Logger

	// LoggerConfig is server logger configuration for Raft logger.
	// Must be either: "LoggerConfig != nil" or "LoggerCore != nil && LoggerWriteSyncer != nil".
	LoggerConfig *zap.Config
}

// NewServer creates a new EtcdServer from the supplied configuration. The
// configuration is considered static for the lifetime of the EtcdServer.
func NewServer(cfg ServerConfig) (*EtcdServer, error) {
	cl, err := NewClusterFromURLsMap(cfg.Logger, cfg.InitialClusterToken, cfg.InitialPeerURLsMap)
	cfg.Logger.Info("After create a new cluster", zap.Any("cluster", cl))
	if err != nil {
		return nil, err
	}
	var n raft.Node
	var id types.ID
	id, n = startNode(cfg, cl)
	srv := &EtcdServer{
		r: *newRaftNode(
			raftNodeConfig{
				Node: n,
				lg:   cfg.Logger,
			},
		),
		id:      id,
		cluster: cl,
		lgMu:    new(sync.RWMutex),
		lg:      cfg.Logger,
		Cfg:     cfg,
	}
	tr := &rafthttp.Transport{
		Logger:    cfg.Logger,
		ID:        id,
		ClusterID: cl.ID(),
		// URLs:      cfg.PeerURLs,
		Raft: srv,
	}
	if err = tr.Start(); err != nil {
		return nil, err
	}
	for _, m := range cl.Members() {
		if m.ID != id {
			cfg.Logger.Info("add peer member",
				zap.Any("member", m),
				zap.Any("peerUrls", m.PeerURLs))
			tr.AddPeer(m.ID, m.PeerURLs)
		}
	}
	srv.r.transport = tr

	return srv, nil
}

func startNode(cfg ServerConfig, cl *RaftCluster) (types.ID, raft.Node) {
	member := cl.MemberByName(cfg.Name)
	return member.ID, raft.StartNode(cfg.Logger)
}

// MarshalLogObject implements zapcore.ObjectMarshaller interface.
func (cfg *ServerConfig) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if cfg == nil {
		return nil
	}

	enc.AddString("Name", cfg.Name)
	enc.AddString("DataDir", cfg.DataDir)
	enc.AddString("ClusterToken", cfg.InitialClusterToken)
	enc.AddString("PeerURLs", cfg.PeerURLs.String())
	enc.AddString("ClientURLs", cfg.ClientURLs.String())
	if err := enc.AddReflected("InitialPeerURLsMap", cfg.InitialPeerURLsMap.URLs()); err != nil {
		return err
	}
	return nil
}

// MarshalLogObject implements zapcore.ObjectMarshaller interface.
func (s *EtcdServer) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if s == nil {
		return nil
	}

	if err := enc.AddObject("Cluster", s.cluster); err != nil {
		return err
	}
	if err := enc.AddObject("Config", &s.Cfg); err != nil {
		return err
	}
	enc.AddUint64("id", uint64(s.id))
	if err := enc.AddReflected("raftNode", s.r); err != nil {
		return err
	}
	return nil
}
