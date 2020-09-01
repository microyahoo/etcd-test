package etcdserver

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/microyahoo/etcd-test/pkg/types"
)

// RaftAttributes represents the raft related attributes of an etcd member.
type RaftAttributes struct {
	// PeerURLs is the list of peers in the raft cluster.
	// PeerURL string `json:"peerURL"`
	PeerURLs []string `json:"peerURLs"`
}

// Attributes represents all the non-raft related attributes of an etcd member.
type Attributes struct {
	Name       string   `json:"name,omitempty"`
	ClientURLs []string `json:"clientURLs,omitempty"`
}

// Member ...
type Member struct {
	lg *zap.Logger

	ID types.ID `json:"id"`
	RaftAttributes
	Attributes
}

// NewMember creates a Member without an ID and generates one based on the
// cluster name, peer URLs, and time. This is used for bootstrapping/adding new member.
func NewMember(lg *zap.Logger, name string, peerURLs types.URLs, clusterName string, now *time.Time) *Member {
	return newMember(lg, name, peerURLs, clusterName, now)
}

func newMember(lg *zap.Logger, name string, peerURLs types.URLs, clusterName string, now *time.Time) *Member {
	m := &Member{
		RaftAttributes: RaftAttributes{
			PeerURLs: peerURLs.StringSlice(),
		},
		Attributes: Attributes{Name: name},
		lg:         lg,
	}

	m.lg.Info("member peerURLs", zap.Any("url", peerURLs.StringSlice()))
	var b []byte
	sort.Strings(m.PeerURLs)
	for _, p := range m.PeerURLs {
		b = append(b, []byte(p)...)
	}

	b = append(b, []byte(clusterName)...)
	if now != nil {
		b = append(b, []byte(fmt.Sprintf("%d", now.Unix()))...)
	}

	hash := sha1.Sum(b)
	m.ID = types.ID(binary.BigEndian.Uint64(hash[:8]))
	return m
}

// Clone ...
func (m *Member) Clone() *Member {
	if m == nil {
		return nil
	}
	mm := &Member{
		ID:             m.ID,
		RaftAttributes: RaftAttributes{},
		Attributes: Attributes{
			Name: m.Name,
		},
	}
	if m.PeerURLs != nil {
		mm.PeerURLs = make([]string, len(m.PeerURLs))
		copy(mm.PeerURLs, m.PeerURLs)
	}
	if m.ClientURLs != nil {
		mm.ClientURLs = make([]string, len(m.ClientURLs))
		copy(mm.ClientURLs, m.ClientURLs)
	}
	return mm
}

// MembersByID implements sort by ID interface
type MembersByID []*Member

func (ms MembersByID) Len() int           { return len(ms) }
func (ms MembersByID) Less(i, j int) bool { return ms[i].ID < ms[j].ID }
func (ms MembersByID) Swap(i, j int)      { ms[i], ms[j] = ms[j], ms[i] }

// MembersByPeerURLs implements sort by peer urls interface
type MembersByPeerURLs []*Member

func (ms MembersByPeerURLs) Len() int { return len(ms) }
func (ms MembersByPeerURLs) Less(i, j int) bool {
	return ms[i].PeerURLs[0] < ms[j].PeerURLs[0]
}
func (ms MembersByPeerURLs) Swap(i, j int) { ms[i], ms[j] = ms[j], ms[i] }
