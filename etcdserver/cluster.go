package etcdserver

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/microyahoo/etcd-test/pkg/types"
	"github.com/microyahoo/etcd-test/raft"
)

// RaftCluster is a list of Members that belong to the same raft cluster
type RaftCluster struct {
	lg *zap.Logger

	localID types.ID
	cid     types.ID

	sync.Mutex // guards the fields below
	members    map[types.ID]*Member
}

func NewClusterFromMembers(lg *zap.Logger, id types.ID, membs []*Member) *RaftCluster {
	c := NewCluster(lg)
	c.cid = id
	for _, m := range membs {
		c.members[m.ID] = m
	}
	return c
}

// NewClusterFromURLsMap creates a new raft cluster using provided urls map.
func NewClusterFromURLsMap(lg *zap.Logger, token string, urlsmap types.URLsMap) (*RaftCluster, error) {
	c := NewCluster(lg)
	for name, urls := range urlsmap {
		m := NewMember(lg, name, urls, token, nil)
		if _, ok := c.members[m.ID]; ok {
			return nil, fmt.Errorf("member exists with identical ID %v", m)
		}
		if uint64(m.ID) == raft.None {
			return nil, fmt.Errorf("cannot use %x as member id", raft.None)
		}
		c.members[m.ID] = m
	}
	c.genID()
	return c, nil
}

func NewCluster(lg *zap.Logger) *RaftCluster {
	return &RaftCluster{
		lg:      lg,
		members: make(map[types.ID]*Member),
	}
}

func (c *RaftCluster) genID() {
	mIDs := c.MemberIDs()
	b := make([]byte, 8*len(mIDs))
	for i, id := range mIDs {
		binary.BigEndian.PutUint64(b[8*i:], uint64(id))
	}
	hash := sha1.Sum(b)
	c.cid = types.ID(binary.BigEndian.Uint64(hash[:8]))
}

func (c *RaftCluster) ID() types.ID { return c.cid }

func (c *RaftCluster) Members() []*Member {
	c.Lock()
	defer c.Unlock()
	var ms MembersByID
	for _, m := range c.members {
		ms = append(ms, m.Clone())
	}
	sort.Sort(ms)
	return []*Member(ms)
}

func (c *RaftCluster) Member(id types.ID) *Member {
	c.Lock()
	defer c.Unlock()
	return c.members[id].Clone()
}

func (c *RaftCluster) MemberIDs() []types.ID {
	c.Lock()
	defer c.Unlock()
	var ids []types.ID
	for _, m := range c.members {
		ids = append(ids, m.ID)
	}
	sort.Sort(types.IDSlice(ids))
	return ids
}

// MemberByName returns a Member with the given name if exists.
// If more than one member has the given name, it will panic.
func (c *RaftCluster) MemberByName(name string) *Member {
	c.Lock()
	defer c.Unlock()
	var memb *Member
	for _, m := range c.members {
		if m.Name == name {
			if memb != nil {
				panic(fmt.Sprintf("two member with same name %s found", name))
			}
			memb = m
		}
	}
	return memb.Clone()
}

// localID types.ID
// cid     types.ID
// sync.Mutex // guards the fields below
// members    map[types.ID]*Member

// MarshalLogObject implements zapcore.ObjectMarshaller interface.
func (s *RaftCluster) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if s == nil {
		return nil
	}

	enc.AddUint64("ClusterID", uint64(s.cid))
	enc.AddUint64("LocalID", uint64(s.localID))
	if err := enc.AddReflected("Members", s.members); err != nil {
		return err
	}
	return nil
}
