package raft

import (
	"context"

	"go.uber.org/zap"

	pb "github.com/microyahoo/etcd-test/raft/raftpb"
)

// Node defines ...
type Node interface {
	// Step advances the state machine using the given message. ctx.Err() will be returned, if any.
	Step(ctx context.Context, msg pb.Message) error

	// Ready returns a channel that returns the current point-in-time state.
	// Users of the Node must call Advance after retrieving the state returned by Ready.
	//
	// NOTE: No committed entries from the next Ready may be applied until all committed entries
	// and snapshots from the previous one have finished.
	Ready() <-chan Ready
}

type node struct {
	readyc chan Ready
	recvc  chan pb.Message

	rn *RawNode
}

func (n *node) Ready() <-chan Ready { return n.readyc }

func (n *node) Step(ctx context.Context, m pb.Message) error {
	return n.step(ctx, m)
}

func (n *node) step(ctx context.Context, m pb.Message) error {
	select {
	case n.recvc <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *node) run() {
	var rd Ready
	var readyc chan Ready
	r := n.rn.raft

	for {
		if n.rn.HasReady() {
			rd = n.rn.readyWithoutAccept()
			readyc = n.readyc
		}
		select {
		case readyc <- rd:
			n.rn.lg.Info("raft.node receive ready", zap.Any("ready", rd))
			n.rn.acceptReady(rd)
		case m := <-n.recvc:
			r.Step(m)
		}
	}
}

func newNode(rn *RawNode) node {
	return node{
		readyc: make(chan Ready),
		recvc:  make(chan pb.Message),
		rn:     rn,
	}
}

func newReady(r *raft) Ready {
	rd := Ready{
		Messages: r.msgs,
	}
	return rd
}

// Ready ...
type Ready struct {
	Messages []pb.Message
}

// StartNode returns a new Node
func StartNode(lg *zap.Logger) Node {
	rn, err := NewRawNode(lg)
	if err != nil {
		panic(err)
	}
	n := newNode(rn)
	go n.run()
	return &n
}
