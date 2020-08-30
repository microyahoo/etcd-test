package embed

import (
	"net/url"
)

// Config holds the arguments for configuring an etcd server.
type Config struct {
	Name   string `json:"name"`
	Dir    string `json:"data-dir"`
	WalDir string `json:"wal-dir"`

	LPUrls, LCUrls []url.URL
	APUrls, ACUrls []url.URL

	InitialCluster      string `json:"initial-cluster"`
	InitialClusterToken string `json:"initial-cluster-token"`
}
