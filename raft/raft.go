package raft

import (
	pb "github.com/microyahoo/etcd-test/raft/raftpb"
)

// None is a placeholder node ID used when there is no leader.
const None uint64 = 0

type raft struct {
	id   uint64
	Term uint64
	msgs []pb.Message

	tick func()
	step stepFunc
}

type stepFunc func(r *raft, m pb.Message) error

func newRaft() *raft {
	return &raft{}
}

func (r *raft) Step(m pb.Message) error {
	r.send(m)
	return nil
}

func (r *raft) send(m pb.Message) {
	r.msgs = append(r.msgs, m)
}
