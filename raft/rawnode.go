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
func NewRawNode(cfg *Config) (*RawNode, error) {
	r := newRaft(cfg)
	return &RawNode{
		raft: r,
		lg:   cfg.Logger,
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
	rn.lg.Info("clear the rawNode's messages", zap.Any("raft.msgs", rn.raft.msgs))
	rn.raft.msgs = nil
}

func (rn *RawNode) HasReady() bool {
	if len(rn.raft.msgs) > 0 {
		return true
	}
	return false
}

// Advance notifies the RawNode that the application has applied and saved progress in the
// last Ready results.
func (rn *RawNode) Advance(rd Ready) {
	// if !IsEmptyHardState(rd.HardState) {
	// 	rn.prevHardSt = rd.HardState
	// }
	rn.raft.advance(rd)
}
