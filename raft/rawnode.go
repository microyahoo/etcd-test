package raft

import (
	"go.uber.org/zap"
)

// RawNode is a thread-unsafe Node.
type RawNode struct {
	raft *raft

	lg *zap.Logger
}

// func NewRawNode(config *Config) (*RawNode, error) {
func NewRawNode(lg *zap.Logger) (*RawNode, error) {
	r := newRaft()
	return &RawNode{
		raft: r,
		lg:   lg,
	}, nil
}

// readyWithoutAccept returns a Ready. This is a read-only operation, i.e. there
// is no obligation that the Ready must be handled.
func (rn *RawNode) readyWithoutAccept() Ready {
	return newReady(rn.raft)
}

// acceptReady is called when the consumer of the RawNode has decided to go
// ahead and handle a Ready. Nothing must alter the state of the RawNode between
// this call and the prior call to Ready().
func (rn *RawNode) acceptReady(rd Ready) {
	rn.raft.msgs = nil
}

func (rn *RawNode) HasReady() bool {
	if len(rn.raft.msgs) > 0 {
		return true
	}
	return false
}
