/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"encoding/binary"
	"net"
	"time"

	"github.com/xujiajundd/ycng/utils/logging"
)

/*
1. Relay Server 管理一组Sessions
   一个Session指一次通话，或者一次文件传输
   同步session管理通话，异步session管理文件传输
   通话结束，或无活动超时，session close，文件传输的异步session全部成员下载完成或按超时close

2. Session有一个8byte id。
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

	QueueSize = 500
)

type OutPacket struct {
	Seqid  int16
	Sbn    uint8
	Esi    uint16
	Iframe bool
	Data   []byte
}

type QueueOut struct {
	Queue []*OutPacket
	Idx   int
}

func NewQueueOut() *QueueOut {
	qo := QueueOut{
		Queue: make([]*OutPacket, QueueSize),
		Idx:   0,
	}

	return &qo
}

func (qo *QueueOut) AddItem(isIFrame bool, payload []byte) {
	packet := &OutPacket{
		Iframe: isIFrame,
		Data:   payload,
	}
	qo.Queue[qo.Idx] = packet
	qo.Idx++
	if qo.Idx >= QueueSize {
		qo.Idx = 0
	}

	packet.Seqid = int16(binary.BigEndian.Uint16(payload[0:2]))
	packet.Sbn = payload[8]
	packet.Esi = binary.BigEndian.Uint16(payload[9:11])
}

func (qo *QueueOut) ProcessNack(nack []byte) (seqid int16, n_tries uint8, isIframe bool, packets [][]byte) {
	packets = nil
	if len(nack) < 4 {
		logging.Logger.Warn("incorrect nack payload size:", len(nack))
		return
	}
	seqid = int16(binary.BigEndian.Uint16(nack[0:2]))
	n_tries = uint8(nack[2])
	block_num := uint8(nack[3])
	var blks_map []uint64
	if block_num > 0 {
		if len(nack) < (4 + 8*int(block_num)) {
			logging.Logger.Warn("incorrect nack payload size:", len(nack))
			return
		}
		blks_map = make([]uint64, block_num)
		for i := 0; i < int(block_num); i++ {
			blks_map[i] = binary.BigEndian.Uint64(nack[4+8*i : 4+8*i+8])
		}
	}

	packets = make([][]byte, 0)
	for i := 0; i < QueueSize; i++ {
		packet := qo.Queue[i]
		if packet != nil && packet.Seqid == seqid {
			isIframe = packet.Iframe
			if block_num == 0 {
				packets = append(packets, packet.Data)
			} else {
				bmap := blks_map[packet.Sbn]
				if packet.Esi < 64 {
					if bmap&(uint64(0x01)<<packet.Esi) == 0 {
						packets = append(packets, packet.Data)
					}
				}
			}
		}
	}

	//logging.Logger.Info("process nack for seq:", seqid, " n_tries:", n_tries, " blk_num:", block_num, " packets:", len(packets))
	return
}

type Participant struct {
	Id              int64        //8 byte participant account id
	UdpAddr         *net.UDPAddr //当前udp地址
	TcpConn         *net.TCPConn //当前tcp连接
	LastActiveTime  time.Time
	Metrics         *Metrics //针对每个participants的in/out metrics
	PendingMsg      *Message
	PendingExtra    *MetrixDataUp
	VideoQueueOut   *QueueOut
	Tseq            int16
	OnlyAcceptAudio bool
}

type Session struct {
	Id           int64
	Type         int
	Participants map[int64]*Participant
}

func NewSession(id int64) *Session {
	session := &Session{
		Id: id,
	}

	return session
}

//待定。。。
type Sessions struct {
	sessions map[int64][]*Session
}

func NewSessions() *Sessions {
	s := &Sessions{
		sessions: make(map[int64][]*Session),
	}
	return s
}
