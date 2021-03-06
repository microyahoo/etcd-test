module github.com/microyahoo/etcd-test

go 1.14

require (
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.4.2
	github.com/sirupsen/logrus v1.4.2
	github.com/soheilhy/cmux v0.1.4
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200716221620-18dfb9cca345 // HEAD of release-3.4, also reproducible on master
	go.uber.org/zap v1.16.0
	google.golang.org/grpc v1.26.0
)
