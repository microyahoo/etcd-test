package transport

import (
	"net"
)

// NewListener creates a new listner.
func NewListener(addr, scheme string) (l net.Listener, err error) {
	if l, err = newListener(addr, scheme); err != nil {
		return nil, err
	}
	// return wrapTLS(scheme, nil, l)
	return l, nil
}

func newListener(addr string, scheme string) (net.Listener, error) {
	if scheme == "unix" || scheme == "unixs" {
		// unix sockets via unix://laddr
		return NewUnixListener(addr)
	}
	return net.Listen("tcp", addr)
}

// func wrapTLS(scheme string, tlsinfo *TLSInfo, l net.Listener) (net.Listener, error) {
// 	if scheme != "https" && scheme != "unixs" {
// 		return l, nil
// 	}
// 	if tlsinfo != nil && tlsinfo.SkipClientSANVerify {
// 		return NewTLSListener(l, tlsinfo)
// 	}
// 	return newTLSListener(l, tlsinfo, checkSAN)
// }
