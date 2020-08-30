package raftpb

const (
	MsgTypeHeartbeat = "01" //心跳
	MsgTypeProp      = "02" //prop消息
)

type Message struct {
	Type string
	From uint64
	To   uint64
	Body string
}
