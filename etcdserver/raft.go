package etcdserver

import (
	"log"
	"time"

	"github.com/microyahoo/etcd-test/raft"
	"github.com/microyahoo/etcd-test/rafthttp"
)

type apply struct {
}

type raftNode struct {
	applyc chan apply
	raftNodeConfig
}

func newRaftNode(cfg raftNodeConfig) *raftNode {
	return &raftNode{
		raftNodeConfig: cfg,
		applyc:         make(chan apply),
	}
}

type raftNodeConfig struct {
	raft.Node
	transport rafthttp.Transporter
}

func (r *raftNode) start() {
	go func() {
		for {
			select {
			case rd := <-r.Ready():
				log.Printf("Receive the ready: %#v\n", rd)
				time.Sleep(5 * time.Second)
			}
		}
	}()
}

func (r *raftNode) apply() chan apply {
	return r.applyc
}
