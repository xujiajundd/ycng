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
	saddr        string
	conn         *net.UDPConn
	subscriberCh chan *ReceivedPacket
}

func NewUdpServer(config *Config, subscriber chan *ReceivedPacket) *UdpServer {
	server := &UdpServer{
		saddr:        config.UdpAddr,
		subscriberCh: subscriber,
	}

	return server
}

func (u *UdpServer) Start() {
	addr, err := net.ResolveUDPAddr("udp4", u.saddr)
	if err != nil {
		logging.Logger.Error("error ResolveUDPAddr")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		logging.Logger.Error("error ListenUDP")
	}

	u.conn = conn

	go u.handleClient()
}

func (u *UdpServer) handleClient() {
	var buf [2048]byte

	for {
		size, addr, err := u.conn.ReadFromUDP(buf[0:])
		if err != nil {
			logging.Logger.Error("error ReadFromUDP ", err)
			continue
		}

		packet := &ReceivedPacket{
			body:        buf[0:size],
			fromUdpAddr: addr,
		}

		u.subscriberCh <- packet
	}
}

func (u *UdpServer) SendPacket(packet []byte, addr *net.UDPAddr) {
	//TODO: 这个线程安全么？
	u.conn.WriteToUDP(packet, addr)
}

func (u *UdpServer) Stop() {
	u.conn.Close()
	u.conn = nil
	u.saddr = ""
	u.subscriberCh = nil
}
