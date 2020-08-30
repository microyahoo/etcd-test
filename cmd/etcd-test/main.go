package main

import (
	"fmt"
	"log"
	// "net/http"
	"os"
	"os/signal"
	// "strconv"
	// "strings"
	// "time"

	// "github.com/gogo/protobuf/proto"
	// etcdpb "go.etcd.io/etcd/etcdserver/etcdserverpb"

	"github.com/microyahoo/etcd-test/embed"
	"github.com/microyahoo/etcd-test/pkg/types"
	_ "github.com/microyahoo/etcd-test/raft/raftpb"
	"github.com/microyahoo/etcd-test/rafthttp"
)

func main() {
	// request := &etcdpb.Request{}
	// fmt.Println(request.String())
	// fmt.Println(proto.CompactTextString(request))

	go func() {
		initialCluster := "server1=http://127.0.0.1:12380,server2=http://127.0.0.1:22380,server3=http://127.0.0.1:32380"
		urlsmap, err := types.NewURLsMap(initialCluster)
		fmt.Printf("**urlMap = %#v\n", urlsmap)
		if err != nil {
			log.Panic(fmt.Sprintf("Failed to create new urls map: %s", err))
		}

		initialAdvertisePeerUrls := "http://127.0.0.1:12380"
		initialClusterToken := "etcd-cluster"
		peerURLs, err := types.NewURLs([]string{initialAdvertisePeerUrls})
		if err != nil {
			log.Panic(fmt.Sprintf("Failed to create new urls: %s", err))
		}
		config := &embed.Config{
			Name:                "server1",
			InitialCluster:      initialCluster,
			InitialClusterToken: initialClusterToken,
			APUrls:              peerURLs,
		}
		e, err := embed.StartEtcd(config)
		if err != nil {
			log.Panic(fmt.Sprintf("Failed to create new server: %s", err))
		}
		log.Printf(fmt.Sprintf("etcd: %#v", e))
	}()

	go func() {
		initialCluster := "server1=http://127.0.0.1:12380,server2=http://127.0.0.1:22380,server3=http://127.0.0.1:32380"
		initialAdvertisePeerUrls := "http://127.0.0.1:22380"
		initialClusterToken := "etcd-cluster"
		peerURLs, err := types.NewURLs([]string{initialAdvertisePeerUrls})
		if err != nil {
			log.Panic(fmt.Sprintf("Failed to create new urls: %s", err))
		}
		config := &embed.Config{
			Name:                "server1",
			InitialCluster:      initialCluster,
			InitialClusterToken: initialClusterToken,
			APUrls:              peerURLs,
		}
		e, err := embed.StartEtcd(config)
		if err != nil {
			log.Panic(fmt.Sprintf("Failed to create new server: %s", err))
		}
		log.Printf(fmt.Sprintf("etcd: %#v", e))
	}()

	go func() {
		initialCluster := "server1=http://127.0.0.1:12380,server2=http://127.0.0.1:22380,server3=http://127.0.0.1:32380"
		initialAdvertisePeerUrls := "http://127.0.0.1:32380"
		initialClusterToken := "etcd-cluster"
		peerURLs, err := types.NewURLs([]string{initialAdvertisePeerUrls})
		if err != nil {
			log.Panic(fmt.Sprintf("Failed to create new urls: %s", err))
		}
		config := &embed.Config{
			Name:                "server1",
			InitialCluster:      initialCluster,
			InitialClusterToken: initialClusterToken,
			APUrls:              peerURLs,
		}
		e, err := embed.StartEtcd(config)
		if err != nil {
			log.Panic(fmt.Sprintf("Failed to create new server: %s", err))
		}
		log.Printf(fmt.Sprintf("etcd: %#v", e))
	}()

	//peerURL1 := "http://127.0.0.1:8081"
	//peerURL2 := "http://127.0.0.1:8082"

	//id1 := getPid(peerURL1)
	//id2 := getPid(peerURL2)

	////开启节点1
	//tr1 := &rafthttp.Transport{}
	//tr1.Start(int64(id1))
	//go func() {
	//	err := http.ListenAndServe(":8081", tr1.Handler())
	//	log.Fatal(err)
	//}()

	////开启节点2
	//tr2 := &rafthttp.Transport{}
	//tr2.Start(int64(id2))
	//go func() {
	//	err := http.ListenAndServe(":8082", tr2.Handler())
	//	log.Fatal(err)
	//}()

	//// time.Sleep(time.Second * 3)

	////节点1添加节点2
	//tr1.AddPeer(int64(id2), peerURL2)

	////节点2添加节点1
	//tr2.AddPeer(int64(id1), peerURL1)

	//go func() {
	//	ticker := time.NewTicker(time.Second * 10)
	//	defer ticker.Stop()
	//	for {
	//		select {
	//		case <-ticker.C:
	//			peers := tr1.GetPeers()
	//			for i := range peers {
	//				peers[i].send(&pb.Message{MsgType: msgTypeProp, MsgBody: "Hello, I am transport 1"})
	//			}
	//		}
	//	}
	//}()

	//go func() {
	//	ticker := time.NewTicker(time.Second * 10)
	//	defer ticker.Stop()
	//	for {
	//		select {
	//		case <-ticker.C:
	//			peers := tr2.GetPeers()
	//			for i := range peers {
	//				peers[i].send(&pb.Message{MsgType: msgTypeProp, MsgBody: "Hello, I am transport 2"})
	//			}
	//		}
	//	}
	//}()

	closeC := rafthttp.NewCloseNotifier()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			fmt.Fprintln(os.Stderr, "etcd stream: received signal:", sig)
			if os.Interrupt == sig {
				//关闭退出
				// tr1.Stop()
				// tr2.Stop()
				closeC.Close()
				os.Exit(1)
			}
		}
	}()

	<-closeC.CloseNotify()
}

// func getPid(purl string) int64 {
// 	index := strings.LastIndex(purl, ":")
// 	if index > 0 {
// 		id, err := strconv.ParseInt(purl[index+1:], 10, 64)
// 		if err != nil {
// 			println(err)
// 		}
// 		return id
// 	}

// 	return 0
// }
