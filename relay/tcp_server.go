/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

type TcpServer struct {

}

func NewTcpServer(config *Config, subscriber chan *ReceivedPacket) *TcpServer {
	server := &TcpServer{
	}

	return server
}

func (t *TcpServer) Start() {

}

func (t *TcpServer) Stop() {

}