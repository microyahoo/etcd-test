package main

import (
	"fmt"
	"os"
	"os/signal"

	"go.uber.org/zap"

	"github.com/microyahoo/etcd-test/embed"
	"github.com/microyahoo/etcd-test/pkg/types"
	_ "github.com/microyahoo/etcd-test/raft/raftpb"
	"github.com/microyahoo/etcd-test/rafthttp"
)

var logger, _ = zap.NewDevelopment()

func main() {
	go func() {
		initialCluster := "server1=http://127.0.0.1:12380,server2=http://127.0.0.1:22380,server3=http://127.0.0.1:32380"
		urlsmap, err := types.NewURLsMap(initialCluster)
		logger.Info("**urlMap", zap.Any("urlMap", urlsmap))
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create new urls map: %s", err))
		}

		initialAdvertisePeerUrls := "http://127.0.0.1:12380"
		initialClusterToken := "etcd-cluster"
		peerURLs, err := types.NewURLs([]string{initialAdvertisePeerUrls})
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create new urls: %s", err))
		}

		config := embed.NewConfig()
		config.Name = "server1"
		config.InitialCluster = initialCluster
		config.InitialClusterToken = initialClusterToken
		config.APUrls = peerURLs
		config.LPUrls = peerURLs
		config.LogOutputs = []string{"logs/server1.log"}

		e, err := embed.StartEtcd(config)
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create new server: %s", err))
		}
		logger.Info("etcd", zap.Any("etcd", e))
		// https://github.com/tchap/zapext/blob/master/types/http_request.go
	}()

	go func() {
		initialCluster := "server1=http://127.0.0.1:12380,server2=http://127.0.0.1:22380,server3=http://127.0.0.1:32380"
		initialAdvertisePeerUrls := "http://127.0.0.1:22380"
		initialClusterToken := "etcd-cluster"
		peerURLs, err := types.NewURLs([]string{initialAdvertisePeerUrls})
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create new urls: %s", err))
		}

		config := embed.NewConfig()
		config.Name = "server2"
		config.InitialCluster = initialCluster
		config.InitialClusterToken = initialClusterToken
		config.APUrls = peerURLs
		config.LPUrls = peerURLs
		config.LogOutputs = []string{"logs/server2.log"}

		e, err := embed.StartEtcd(config)
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create new server: %s", err))
		}
		logger.Info("etcd", zap.Any("etcd", e))
	}()

	go func() {
		initialCluster := "server1=http://127.0.0.1:12380,server2=http://127.0.0.1:22380,server3=http://127.0.0.1:32380"
		initialAdvertisePeerUrls := "http://127.0.0.1:32380"
		initialClusterToken := "etcd-cluster"
		peerURLs, err := types.NewURLs([]string{initialAdvertisePeerUrls})
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create new urls: %s", err))
		}

		config := embed.NewConfig()
		config.Name = "server3"
		config.InitialCluster = initialCluster
		config.InitialClusterToken = initialClusterToken
		config.APUrls = peerURLs
		config.LPUrls = peerURLs
		config.LogOutputs = []string{"logs/server3.log"}

		e, err := embed.StartEtcd(config)
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create new server: %s", err))
		}
		logger.Info("etcd", zap.Any("etcd", e))
	}()

	closeC := rafthttp.NewCloseNotifier()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			logger.Warn("etcd stream: received signal", zap.Any("Signal", sig))
			if os.Interrupt == sig {
				closeC.Close()
				os.Exit(1)
			}
		}
	}()

	<-closeC.CloseNotify()
}
