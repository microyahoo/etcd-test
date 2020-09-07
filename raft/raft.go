package raft

import (
	"go.uber.org/zap"

	pb "github.com/microyahoo/etcd-test/raft/raftpb"
)

// None is a placeholder node ID used when there is no leader.
const None uint64 = 0

// Config contains the parameters to start a raft.
type Config struct {
	ID     uint64
	Logger *zap.Logger
}

type raft struct {
	id   uint64
	Term uint64
	msgs []pb.Message

	tick func()
	step stepFunc

	logger *zap.Logger
}

type stepFunc func(r *raft, m pb.Message) error

func newRaft(cfg *Config) *raft {
	return &raft{
		logger: cfg.Logger,
	}
}

func (r *raft) Step(m pb.Message) error {
	r.send(m)
	return nil
}

func (r *raft) send(m pb.Message) {
	r.msgs = append(r.msgs, m)
}

func (r *raft) advance(rd Ready) {
	r.logger.Info("reduce uncommitted size, update cursor for next Ready, update raftLog", zap.Any("ready", rd))
}
