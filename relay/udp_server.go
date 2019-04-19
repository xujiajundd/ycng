/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"net"
	"github.com/xujiajundd/ycng/utils/logging"
)

type UdpServer struct {
    saddr string
    conn *net.UDPConn
}


func NewUdpServer(config *Config) *UdpServer {
	server := &UdpServer{
		saddr: config.UdpAddr,
	}

	return server
}


func (u *UdpServer) Start() {
	addr, err := net.ResolveUDPAddr("udp4", u.saddr)
	if err != nil {
		logging.Logger.Error("")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {

	}

	u.conn = conn


}

func (u *UdpServer) handleClient() {
	var buf [1024]byte

	for {
		size, addr, err := u.conn.ReadFromUDP(buf[0:])
		if err != nil {

		}
	}
}

func (u *UdpServer) Stop() {

}

