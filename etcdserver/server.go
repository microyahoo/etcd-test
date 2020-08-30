package etcdserver

import (
	"context"
	"fmt"
	"log"
	"net/http"

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
}

var _ ServerPeer = (*EtcdServer)(nil)
var _ rafthttp.Raft = (*EtcdServer)(nil)

func (s *EtcdServer) Process(ctx context.Context, m pb.Message) error {
	return s.r.Step(ctx, m)
}

func (s *EtcdServer) Start() {
	log.Println("Start to run etcdserver")
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
			log.Printf("Receive apply: %#v\n", ap)
		}
	}
}

func (s *EtcdServer) RaftHandler() http.Handler { return s.r.transport.Handler() }

// ServerConfig holds the configuration of etcd as taken from the command line or discovery.
type ServerConfig struct {
	Name string
	// ClientURLs types.URLs
	PeerURLs types.URLs

	InitialPeerURLsMap  types.URLsMap
	InitialClusterToken string
}

func NewServer(cfg ServerConfig) (*EtcdServer, error) {
	cl, err := NewClusterFromURLsMap(cfg.InitialClusterToken, cfg.InitialPeerURLsMap)
	fmt.Printf("**cluster = %#v\n", cl)
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
			},
		),
		id:      id,
		cluster: cl,
	}
	tr := &rafthttp.Transport{
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
			fmt.Printf("****member: %#v, peerUrls: %#v\n", m, m.PeerURLs)
			tr.AddPeer(m.ID, m.PeerURLs)
		}
	}
	srv.r.transport = tr

	return srv, nil
}

func startNode(cfg ServerConfig, cl *RaftCluster) (types.ID, raft.Node) {
	member := cl.MemberByName(cfg.Name)
	return member.ID, raft.StartNode()
}
