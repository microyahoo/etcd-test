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

	// Advance notifies the Node that the application has saved progress up to the last Ready.
	// It prepares the node to return the next available Ready.
	//
	// The application should generally call Advance after it applies the entries in last Ready.
	//
	// However, as an optimization, the application may call Advance while it is applying the
	// commands. For example. when the last Ready contains a snapshot, the application might take
	// a long time to apply the snapshot data. To continue receiving Ready without blocking raft
	// progress, it can call Advance before finishing applying the last ready.
	Advance()
}

type node struct {
	readyc   chan Ready
	recvc    chan pb.Message
	advancec chan struct{}

	rn *RawNode
}

func (n *node) Ready() <-chan Ready { return n.readyc }

func (n *node) Step(ctx context.Context, m pb.Message) error {
	return n.step(ctx, m)
}

func (n *node) Advance() {
	select {
	case n.advancec <- struct{}{}:
		// case <-n.done:
	}
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
	var advancec chan struct{}

	r := n.rn.raft

	for {
		if advancec != nil {
			readyc = nil
		} else if n.rn.HasReady() {
			// Populate a Ready. Note that this Ready is not guaranteed to
			// actually be handled. We will arm readyc, but there's no guarantee
			// that we will actually send on it. It's possible that we will
			// service another channel instead, loop around, and then populate
			// the Ready again. We could instead force the previous Ready to be
			// handled first, but it's generally good to emit larger Readys plus
			// it simplifies testing (by emitting less frequently and more
			// predictably).
			rd = n.rn.readyWithoutAccept()
			readyc = n.readyc
		}
		select {
		case readyc <- rd:
			n.rn.lg.Info("raft.node receive ready", zap.Any("ready", rd))
			n.rn.acceptReady(rd)
			advancec = n.advancec
		case <-advancec:
			n.rn.lg.Info("receive the advance notify from etcd server")
			n.rn.Advance(rd)
			rd = Ready{}
			advancec = nil
		case m := <-n.recvc:
			r.Step(m)
		}
	}
}

func newNode(rn *RawNode) node {
	return node{
		readyc:   make(chan Ready),
		recvc:    make(chan pb.Message),
		advancec: make(chan struct{}),
		rn:       rn,
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
func StartNode(cfg *Config) Node {
	rn, err := NewRawNode(cfg)
	if err != nil {
		panic(err)
	}
	n := newNode(rn)
	go n.run()
	return &n
}
