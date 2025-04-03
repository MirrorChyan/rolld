package utils

import "net"

func RandomAvailablePort() int {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	defer func(l net.Listener) {
		_ = l.Close()
	}(l)
	return l.Addr().(*net.TCPAddr).Port
}
