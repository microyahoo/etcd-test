package embed

import (
	"net/url"
)

// Config holds the arguments for configuring an etcd server.
type Config struct {
	Name   string `json:"name"`
	Dir    string `json:"data-dir"`
	WalDir string `json:"wal-dir"`

	LPUrls, LCUrls []url.URL // listen-peer-urls and listen-client-urls
	APUrls, ACUrls []url.URL // initial-advertise-peer-urls and advertise-client-urls

	InitialCluster      string `json:"initial-cluster"`
	InitialClusterToken string `json:"initial-cluster-token"`
}
