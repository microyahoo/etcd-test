package etcdserver

import (
	"time"

	"go.uber.org/zap"

	"github.com/microyahoo/etcd-test/raft"
	"github.com/microyahoo/etcd-test/rafthttp"
)

type apply struct {
}

type raftNode struct {
	lg *zap.Logger

	applyc chan apply
	raftNodeConfig
}

func newRaftNode(cfg raftNodeConfig) *raftNode {
	return &raftNode{
		lg:             cfg.lg,
		raftNodeConfig: cfg,
		applyc:         make(chan apply),
	}
}

type raftNodeConfig struct {
	lg *zap.Logger
	raft.Node
	transport rafthttp.Transporter
}

func (r *raftNode) start() {
	go func() {
		for {
			select {
			case rd := <-r.Ready():
				r.lg.Info("Receive the ready, handle the messages", zap.Any("ready", rd))
				time.Sleep(15 * time.Second)
				r.lg.Info("Already handle the messages, notify the node", zap.Any("ready", rd))
				r.Advance()
			}
		}
	}()
}

func (r *raftNode) apply() chan apply {
	return r.applyc
}
