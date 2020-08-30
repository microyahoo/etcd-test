package rafthttp

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
		log.Panicf("failed new urls: %#v", peerURLs)
	}
	peer := startPeer(tr, urls, id)

	tr.mu.Lock()
	tr.peers[id] = peer
	tr.mu.Unlock()
}

func (tr *Transport) Handler() http.Handler {
	streamHandler := newStreamHandler(tr, tr.ID, tr.ClusterID)
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
	tr  *Transport //关联的rafthttp.Transport实例
	id  types.ID   //当前节点ID
	cid types.ID   //当前集群ID
}

func newStreamHandler(tr *Transport, id, cid types.ID) http.Handler {
	return &streamHandler{
		tr:  tr,
		id:  id,
		cid: cid,
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

	log.Printf("Stream handler: %#v, %s", r.URL.String(), r.Header)
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
	r := t.Raft
	pr := &peer{
		localID: t.ID,
		id:      peerID,
		r:       r,
		writer:  newStreamWriter(t.ID, peerID),
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
				log.Panicf("failed to process message: %s", err)
			}
			// select {
			// case pr.writer.msgc <- msg:
			// default:
			// 	log.Printf("write to writer error msg is %v", msg)
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
		log.Printf("attachOutgoingConn error")
	}
}

type streamWriter struct {
	localID types.ID //本端的ID
	peerID  types.ID //对端节点的ID

	closer io.Closer //负责关闭底层的长连接

	mu sync.Mutex

	enc   *messageEncoder
	msgc  chan pb.Message    //Peer会将待发送的消息写入到该通道，streamWriter则从该通道中读取消息并发送出去
	connc chan *outgoingConn //通过该通道获取当前streamWriter实例关联的底层网络连接，  outgoingConn其实是对网络连接的一层封装，其中记录了当前连接使用的协议版本，以及用于关闭连接的Flusher和Closer等信息。
	stopc chan struct{}
}

func newStreamWriter(localID, peerID types.ID) *streamWriter {
	sw := &streamWriter{
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

		flusher http.Flusher //负责刷新底层连接，将数据真正发送出去

		msgc chan pb.Message
	)

	tickc := time.NewTicker(time.Second * 7) //发送心跳的定时器
	defer tickc.Stop()

	for {
		select {
		case msg := <-msgc:
			err := sw.enc.encode(&msg)
			if err != nil {
				log.Printf("Send to peer peerID is %d fail, error is: %v", sw.peerID, err)
			} else {
				flusher.Flush()
				log.Printf("Succeed to send message to peer peerID: %d, MsgType is: %s, MsgBody is: %s",
					sw.peerID, msg.Type, msg.Body)
			}

		case <-heartbeatC: //向对端发送心跳消息
			err := sw.enc.encode(&pb.Message{
				Type: pb.MsgTypeHeartbeat,
				Body: time.Now().Format("2006-01-02 15:04:05"),
			})
			if err != nil {
				log.Printf("Send to peer heartbeat data fail, peerID is %d, error is %v",
					sw.peerID, err)
			} else {
				flusher.Flush()
				log.Printf("Succeed to send heartbeat data to peer peerID: %d", sw.peerID)
			}
		case conn := <-sw.connc:
			log.Printf("Connection %#v is comming...", conn)
			sw.enc = &messageEncoder{w: conn.Writer}
			flusher = conn.Flusher
			sw.closer = conn.Closer

			heartbeatC, msgc = tickc.C, sw.msgc

		case <-sw.stopc:
			log.Println("msgWriter stop!")
			sw.closer.Close()
			return
		}
	}
}

func (sw *streamWriter) stop() {
	close(sw.stopc)
}

type streamReader struct {
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
	}
	go sr.run()

	return sr
}

func (sr *streamReader) run() {
	// time.Sleep(time.Second * 5)
	for {
		readCloser, err := sr.dial()
		if err != nil {
			log.Printf("Dial peer error, peerID is %d, err is: %v", sr.peerID, err)
			time.Sleep(time.Second * 10)
			continue
		}
		sr.closer = readCloser

		err = sr.decodeLoop(readCloser)
		if err != nil {
			log.Printf("DecodeLoop error, peerID is %d, error is %v", sr.peerID, err)
		}
		sr.closer.Close()
	}
}

func (sr *streamReader) dial() (io.ReadCloser, error) {
	u := sr.picker.pick()
	req, err := http.NewRequest("GET", u.String()+"/raft/stream/dial", nil)
	// req, err := http.NewRequest("GET", sr.peerURL+"/raft/stream/dial", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("PeerID", fmt.Sprintf("%d", sr.localID))

	log.Printf("Start to dial peer: %s", req.URL.String())
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
			log.Printf("\t**Read decodeLoop error, peerID is %d, err is %v", sr.peerID, err)
			continue
		}

		log.Printf("\t**Read from peer MsgType is %s, MsgBody is %s", msg.Type, msg.Body)
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

func getPid(purl string) int64 {
	index := strings.LastIndex(purl, ":")
	if index > 0 {
		id, err := strconv.ParseInt(purl[index+1:], 10, 64)
		if err != nil {
			println(err)
		}
		return id
	}

	return 0
}
