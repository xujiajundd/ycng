/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"net"
	"time"
)

/*
1. Relay Server 管理一组Sessions
   一个Session指一次通话，或者一次文件传输
   同步session管理通话，异步session管理文件传输
   通话结束，或无活动超时，session close，文件传输的异步session全部成员下载完成或按超时close

2. Session有一个32byteid，和一个8byte short id。
   Session有参与者，通话中参与者就是各方，文件传输参与者就是文件接收者。
   异步Session有文件缓存地址

3. 参与者
   在同步通话中，参与这通过REG注册进来，包含其网络地址
   异步文件时，参与这为用户ID

4. UDP，也可能UDP over TCP

*/

const (
	SessionTypeRealtimeCall         = 0
	SessionTypeRealtimeFileTransfer = 1
	SessionTypeAsyncFileTransfer    = 2
)

type Participant struct {
	Id             uint64       //8 byte participant account id
	UdpAddr        *net.UDPAddr //当前udp地址
	TcpConn        *net.TCPConn //当前tcp连接
	LastActiveTime time.Time
	Metrics        *Metrics //针对每个participants的in/out metrics
	PendingMsg     *Message
	Tseq           int16
}

type Session struct {
	Id           []byte
	IdShort      uint64
	Type         int
	Participants map[uint64]*Participant
}

func NewSession(id uint64) *Session {
	session := &Session{
		IdShort: id,
	}

	return session
}

//待定。。。
type Sessions struct {
	sessions map[uint64][]*Session
}

func NewSessions() *Sessions {
	s := &Sessions{
		sessions: make(map[uint64][]*Session),
	}
	return s
}

func (s *Sessions) AddSession(session *Session) {

}

func (s *Sessions) FindSession(sid uint64, from uint64) *Session {
	ss := s.sessions[sid]
	if ss == nil || len(ss) == 0 {
		return nil
	}
	if len(ss) == 1 {
		return ss[0]
	} else {
		for _, s := range ss {
			if s.Participants[from] != nil {
				return s
			}
		}
		return nil
	}
}
