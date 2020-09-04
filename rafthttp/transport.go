package rafthttp

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/microyahoo/etcd-test/pkg/types"
	pb "github.com/microyahoo/etcd-test/raft/raftpb"
)

const (
	recvBufSize   = 4096
	streamBufSize = 4096
)

type Raft interface {
	Process(ctx context.Context, m pb.Message) error
}

type Transporter interface {
	// Start starts the given Transporter.
	// Start MUST be called before calling other functions in the interface.
	Start() error

	// Handler returns the HTTP handler of the transporter.
	// A transporter HTTP handler handles the HTTP requests
	// from remote peers.
	// The handler MUST be used to handle RaftPrefix(/raft)
	// endpoint.
	Handler() http.Handler

	// Send sends out the given messages to the remote peers.
	// Each message has a To field, which is an id that maps
	// to an existing peer in the transport.
	// If the id cannot be found in the transport, the message
	// will be ignored.
	Send(m []pb.Message)

	// AddPeer adds a peer with given peer urls into the transport.
	// It is the caller's responsibility to ensure the urls are all valid,
	// or it panics.
	// Peer urls are used to connect to the remote peer.
	AddPeer(id types.ID, urls []string)

	// Stop closes the connections and stops the transporter.
	Stop()
}

type Transport struct {
	Logger *zap.Logger

	ClusterID types.ID
	ID        types.ID // local member ID  当前节点自己的ID

	streamRt http.RoundTripper

	mu    sync.RWMutex
	peers map[types.ID]*peer // peers map

	Raft Raft
}

var _ Transporter = (*Transport)(nil)

func (tr *Transport) Start() error {
	// func (tr *Transport) Start(id int64) error {
	// tr.ID = id
	tr.streamRt = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 10 * time.Second,
			// value taken from http.DefaultTransport
			KeepAlive: 30 * time.Second,
		}).Dial,
	}
	tr.peers = make(map[types.ID]*peer)

	return nil
}

func (tr *Transport) Send(m []pb.Message) {
}

func (tr *Transport) GetPeers() (result []*peer) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	for k := range tr.peers {
		result = append(result, tr.peers[k])
	}

	return
}

// AddPeer(id types.ID, urls []string)
func (tr *Transport) AddPeer(id types.ID, peerURLs []string) {
	tr.mu.RLock()
	if _, ok := tr.peers[id]; ok {
		tr.mu.RUnlock()
		return
	}

	tr.mu.RUnlock()

	urls, err := types.NewURLs(peerURLs)
	if err != nil {
		tr.Logger.Panic("invalid urls", zap.Any("peerURLs", peerURLs))
	}
	peer := startPeer(tr, urls, id)

	tr.mu.Lock()
	tr.peers[id] = peer
	tr.mu.Unlock()
}

func (tr *Transport) Handler() http.Handler {
	streamHandler := newStreamHandler(tr, tr.Raft, tr.ID, tr.ClusterID)
	mux := http.NewServeMux()
	mux.Handle("/raft/stream/", streamHandler)
	return mux
}

func (tr *Transport) Stop() {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	for _, v := range tr.peers {
		v.stop()
	}
	tr.peers = nil
}

type streamHandler struct {
	lg  *zap.Logger
	tr  *Transport //关联的rafthttp.Transport实例
	id  types.ID   //当前节点ID
	cid types.ID   //当前集群ID
	r   Raft
}

func newStreamHandler(tr *Transport, r Raft, id, cid types.ID) http.Handler {
	return &streamHandler{
		lg:  tr.Logger,
		tr:  tr,
		id:  id,
		cid: cid,
		r:   r,
	}
}

func (h *streamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//请求参数校验，如Method是否是GET，检验集群ID
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.Header.Get("PeerID")
	if id == "" {
		w.Header().Set("PeerID", "Must")
		http.Error(w, "PeerID is not allow empty", http.StatusMethodNotAllowed)
		return
	}

	pid, _ := strconv.ParseUint(id, 10, 64)

	p, ok := h.tr.peers[types.ID(pid)]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("Peer '%d' is not in transport", pid)))
		time.Sleep(time.Second * 5)
		return
	}

	h.lg.Info("Stream handler", zap.String("request.URL",
		r.URL.String()), zap.Any("request.Header", r.Header))
	w.WriteHeader(http.StatusOK) //返回状态码 200

	w.(http.Flusher).Flush() //调用Flush()方法将响应数据发送到对端节点

	c := NewCloseNotifier()
	conn := &outgoingConn{ //创建outgoingConn实例
		Writer:  w,
		Flusher: w.(http.Flusher),
		localID: h.tr.ID,
		Closer:  c,
		peerID:  h.id,
	}
	p.attachOutgoingConn(conn) //建立连接, 将outgoingConn实例与对应的streamWriter实例绑定
	<-c.CloseNotify()
}

type CloseNotifier struct {
	done chan struct{}
}

func NewCloseNotifier() *CloseNotifier {
	return &CloseNotifier{
		done: make(chan struct{}),
	}
}

func (n *CloseNotifier) Close() error {
	close(n.done)
	return nil
}

func (n *CloseNotifier) CloseNotify() <-chan struct{} { return n.done }

type peer struct {
	lg *zap.Logger

	localID types.ID //当前节点ID
	// id of the remote raft peer node
	id types.ID //该peer实例对应的节点ID，对端ID

	writer *streamWriter //负责向Stream消息通道中写消息

	msgAppReader *streamReader //负责从Stream消息通道中读消息

	// msgc chan *Message
	recvc  chan pb.Message
	cancel context.CancelFunc // cancel pending works in go routine created by peer.
	stopc  chan struct{}

	r Raft
}

func startPeer(t *Transport, peerUrls types.URLs, peerID types.ID) *peer {
	if t.Logger != nil {
		t.Logger.Info("starting remote peer", zap.String("remote-peer-id", peerID.String()))
	}
	defer func() {
		if t.Logger != nil {
			t.Logger.Info("started remote peer", zap.String("remote-peer-id", peerID.String()))
		}
	}()
	r := t.Raft
	pr := &peer{
		lg:      t.Logger,
		localID: t.ID,
		id:      peerID,
		r:       r,
		writer:  newStreamWriter(t.Logger, t.ID, peerID),
		recvc:   make(chan pb.Message, recvBufSize),
		// msgc:         make(chan *Message, recvBufSize),
		stopc: make(chan struct{}),
	}
	pr.msgAppReader = newStreamReader(t.ID, pr, peerUrls, t)

	ctx, cancel := context.WithCancel(context.Background())
	pr.cancel = cancel
	go func() {
		for msg := range pr.recvc {
			if err := r.Process(ctx, msg); err != nil {
				t.Logger.Warn("failed to process Raft message", zap.Error(err))
			}
			// select {
			// case pr.writer.msgc <- msg:
			// default:
			// 	logger.Infof("write to writer error msg is %v", msg)
			// }
		}
	}()

	return pr
}

func (pr *peer) stop() {
	close(pr.stopc)
}

func (pr *peer) send(msg pb.Message) bool {
	select {
	case pr.recvc <- msg:
		return true
	default:
		return false
	}
}

func (pr *peer) attachOutgoingConn(conn *outgoingConn) {
	select {
	case pr.writer.connc <- conn:

	default:
		pr.lg.Info("attachOutgoingConn error")
	}
}

type streamWriter struct {
	lg *zap.Logger

	localID types.ID //本端的ID
	peerID  types.ID //对端节点的ID

	closer io.Closer //负责关闭底层的长连接

	mu sync.Mutex

	enc   *messageEncoder
	msgc  chan pb.Message    //Peer会将待发送的消息写入到该通道，streamWriter则从该通道中读取消息并发送出去
	connc chan *outgoingConn //通过该通道获取当前streamWriter实例关联的底层网络连接，  outgoingConn其实是对网络连接的一层封装，其中记录了当前连接使用的协议版本，以及用于关闭连接的Flusher和Closer等信息。
	stopc chan struct{}
}

func newStreamWriter(lg *zap.Logger, localID, peerID types.ID) *streamWriter {
	sw := &streamWriter{
		lg:      lg,
		localID: localID,
		peerID:  peerID,
		msgc:    make(chan pb.Message, streamBufSize),
		connc:   make(chan *outgoingConn),
		stopc:   make(chan struct{}),
	}

	go sw.run()
	return sw
}

func (sw *streamWriter) writec() chan<- pb.Message {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.msgc
}

func (sw *streamWriter) run() {
	var (
		heartbeatC <-chan time.Time
		flusher    http.Flusher //负责刷新底层连接，将数据真正发送出去
		msgc       chan pb.Message
	)

	tickc := time.NewTicker(time.Second * 7) //发送心跳的定时器
	defer tickc.Stop()

	if sw.lg != nil {
		sw.lg.Info(
			"started stream writer with remote peer",
			zap.String("local-member-id", sw.localID.String()),
			zap.String("remote-peer-id", sw.peerID.String()),
		)
	}

	for {
		select {
		case msg := <-msgc:
			err := sw.enc.encode(&msg)
			if err != nil {
				sw.lg.Warn("Failed to send to peer peerID",
					zap.String("remote-peer-id", sw.peerID.String()), zap.Error(err))
			} else {
				flusher.Flush()
				sw.lg.Info("Succeed to send message to peer peerID",
					zap.String("message-type", msg.Type),
					zap.String("message-body", msg.Body),
					zap.String("remote-peer-id", sw.peerID.String()))
			}

		case <-heartbeatC: //向对端发送心跳消息
			err := sw.enc.encode(&pb.Message{
				Type: pb.MsgTypeHeartbeat,
				Body: fmt.Sprintf("%s-%d", time.Now().Format("2006-01-02 15:04:05"), rand.Int31()),
			})
			if err != nil {
				sw.lg.Warn("Failed to send to peer peerID",
					zap.String("remote-peer-id", sw.peerID.String()), zap.Error(err))
			} else {
				flusher.Flush()
				sw.lg.Info("Succeed to send heartbeat data to peer peerID",
					zap.String("remote-peer-id", sw.peerID.String()))
			}
		case conn := <-sw.connc:
			sw.lg.Info("Connection is comming...", zap.Any("connection", conn))
			sw.enc = &messageEncoder{w: conn.Writer}
			flusher = conn.Flusher
			sw.closer = conn.Closer

			heartbeatC, msgc = tickc.C, sw.msgc

		case <-sw.stopc:
			sw.lg.Info("msgWriter stop!")
			sw.closer.Close()
			return
		}
	}
}

func (sw *streamWriter) stop() {
	close(sw.stopc)
}

type streamReader struct {
	lg *zap.Logger

	localID types.ID
	peerID  types.ID   //对端节点的ID
	tr      *Transport //关联的rafthttp.Transport实例
	// peerURL string     //对端URL

	picker *urlPicker
	recvc  chan<- pb.Message

	mu     sync.Mutex
	closer io.Closer //负责关闭底层的长连接

	done chan struct{}
}

func newStreamReader(localID types.ID, pr *peer, peerURLs types.URLs, tr *Transport) *streamReader {
	picker := newURLPicker(peerURLs)
	sr := &streamReader{
		localID: localID,
		peerID:  pr.id,
		// peerURL: peerURL,
		picker: picker,
		recvc:  pr.recvc,
		tr:     tr,
		done:   make(chan struct{}),
		lg:     tr.Logger,
	}
	go sr.run()

	return sr
}

func (sr *streamReader) run() {
	for {
		readCloser, err := sr.dial()
		if err != nil {
			sr.lg.Warn("Dial peer error",
				zap.String("remote-peer-id", sr.peerID.String()),
				zap.Error(err))
			time.Sleep(time.Second * 10)
			continue
		}
		sr.closer = readCloser

		err = sr.decodeLoop(readCloser)
		if err != nil {
			sr.lg.Warn("Decode loop error",
				zap.String("remote-peer-id", sr.peerID.String()),
				zap.Error(err))
		}
		sr.closer.Close()
	}
}

func (sr *streamReader) dial() (io.ReadCloser, error) {
	u := sr.picker.pick()
	req, err := http.NewRequest("GET", u.String()+"/raft/stream/dial", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("PeerID", fmt.Sprintf("%d", sr.localID))

	sr.lg.Info("Start to dial peer", zap.String("request-url", req.URL.String()),
		zap.Any("request-header", req.Header))
	resp, err := sr.tr.streamRt.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (sr *streamReader) decodeLoop(rc io.ReadCloser) error {
	dec := &messageDecoder{rc}
	for {
		msg, err := dec.decode()
		if err != nil {
			sr.lg.Warn("Read decodeLoop error",
				zap.String("remote-peer-id", sr.peerID.String()),
				zap.Error(err))
			continue
		}

		sr.lg.Info("Read from peer",
			zap.String("message-type", msg.Type),
			zap.String("message-body", msg.Body),
			zap.String("remote-peer-id", sr.peerID.String()))
		recvc := sr.recvc
		select {
		case recvc <- msg:
		default:
		}
	}
}

type outgoingConn struct {
	io.Writer
	http.Flusher
	io.Closer
	localID types.ID
	peerID  types.ID
}

type messageEncoder struct {
	w io.Writer
}

func (m *messageEncoder) encode(msg *pb.Message) error {
	byts, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	dataLen := make([]byte, 8)
	binary.BigEndian.PutUint64(dataLen, uint64(len(byts)))

	sendData := append(dataLen, byts...)
	_, err = m.w.Write(sendData)
	if err != nil {
		return err
	}

	return nil
}

type messageDecoder struct {
	r io.Reader
}

func (dec *messageDecoder) decode() (pb.Message, error) {
	var m pb.Message
	var l uint64
	if err := binary.Read(dec.r, binary.BigEndian, &l); err != nil {
		return m, err
	}

	buf := make([]byte, int(l))
	if _, err := io.ReadFull(dec.r, buf); err != nil {
		return m, err
	}
	err := json.Unmarshal(buf, &m)
	if err != nil {
		return m, err
	}
	return m, nil
}
