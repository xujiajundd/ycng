/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"encoding/binary"
	"errors"
	"net"

	"github.com/xujiajundd/ycng/utils"
)

/*
relay需要解混淆和加混淆
relay需要的信息：是否i帧？
*/

const (
	UdpMessageTypeNoop              = 0  //NOOP
	UdpMessageTypeTurnReg           = 1  //客户端注册一个session的请求
	UdpMessageTypeTurnRegReceived   = 2  //服务器返回请求收到
	UdpMessageTypeTurnUnReg         = 3  //客户端注销session注册
	UdpMessageTypeTurnRegNoExist    = 4  //session未注册
	UdpMessageTypeTurnInfo          = 5  //1-1时，回复给各方的外网地址
	UdpMessageTypeTurnProbe         = 6  //p2p探测包
	UdpMessageTypeTurnProbeAck      = 7  //p2p探测回复包
	UdpMessageTypeAudioStream       = 20 //音频包
	UdpMessageTypeVideoStream       = 30 //视频包
	UdpMessageTypeVideoStreamIFrame = 31 //视频i帧
	UdpMessageTypeVideoNack         = 32 //视频请求重发包
	UdpMessageTypeVideoAskForIFrame = 33 //视频请求i帧
	UdpMessageTypeVideoOnlyIFrame   = 34 //视频只收i帧
	UdpMessageTypeVideoOnlyAudio    = 35 //视频只收音频

	UdpMessageTypeUserReg         = 200 //注册一个客户端
	UdpMessageTypeUserRegReceived = 201
	UdpMessageTypeUserSignal      = 202 //通过UDP来转发的信令，信令统一在push中定义
)

const (
	UdpMessageFlagExtra = 1 << 0
	UdpMessageFlagDest  = 1 << 1
)

const (
	UdpMessageExtraTypeMetrix = 1

	YCKMetrixDataTypeUp   = 2
)

type Message struct {
	Tseq      int16
	Tid       byte
	Timestamp uint16
	Version   uint16
	Flags     uint16
	MsgType   uint8
	From      uint64
	To        uint64
	Dest      uint64
	Payload   []byte
	Extra     []byte
}

type ReceivedPacket struct {
	FromUdpAddr *net.UDPAddr
	FromTcpConn *net.TCPConn
	Body        []byte
	Time        int64
}

func NewMessage(msgType uint8, from uint64, to uint64, dest uint64, payload []byte, extra []byte) *Message {
	msg := &Message{
		Tseq:      0,
		Tid:       0,
		Timestamp: 0,
		Version:   1,
		Flags:     0,
		MsgType:   msgType,
		From:      from,
		To:        to,
		Dest:      dest,
		Payload:   payload,
		Extra:     extra,
	}

	if dest != 0 {
		msg.SetFlag(UdpMessageFlagDest)
	}

	if extra != nil {
		msg.SetFlag(UdpMessageFlagExtra)
	}

	return msg
}

func NewMessageFromObfuscatedData(obf []byte) (*Message, error) {
	message := &Message{}
	data := utils.DataFromObfuscated(obf)
	err := message.Unmarshal(data)

	if err != nil {
		return nil, err
	}

	return message, nil
}

func (m *Message) ObfuscatedDataOfMessage() []byte {
	data := m.Marshal()
	obf := utils.ObfuscateData(data)

	return obf
}

func (m *Message) Unmarshal(data []byte) error {
	len := len(data)
	p := 0

	if len < 26 {
		return errors.New("incorrect packet, len < 26")
	}
	m.Tseq = int16(binary.BigEndian.Uint16(data[p : p+2]))
	p += 2

	m.Tid = data[p]
	p += 1

	m.Timestamp = binary.BigEndian.Uint16(data[p : p+2])
	p += 2

	versionAndFlags := binary.BigEndian.Uint16(data[p : p+2])
	p += 2
	m.Version = (versionAndFlags & 0xf000) >> 12
	m.Flags = versionAndFlags & 0xfff

	m.MsgType = data[p]
	p += 1

	if len >= p+8 {
		m.From = binary.BigEndian.Uint64(data[p : p+8])
		p += 8
	}
	if len >= p+8 {
		m.To = binary.BigEndian.Uint64(data[p : p+8])
		p += 8
	}

	if m.HasFlag(UdpMessageFlagDest) {
		if len >= p+8 {
			m.Dest = binary.BigEndian.Uint64(data[p : p+8])
			p += 8
		}
	}

	var payloadLen uint16
	if len >= p+2 {
		payloadLen = binary.BigEndian.Uint16(data[p : p+2])
		p += 2
	}

	if len >= p+int(payloadLen) {
		//TODO; 这个地方是copy呢？还是直接这样呢? 貌似不copy也是可以的。
		m.Payload = data[p : p+int(payloadLen)]
		//m.Payload = make([]byte,payloadLen)
		//copy(m.Payload, data[p : p+int(payloadLen)])
		p += int(payloadLen)
	} else {
		return errors.New("incorrect packet len for Payload")
	}

	if m.HasFlag(UdpMessageFlagExtra) {
		var extraLen uint16
		if len >= p+2 {
			extraLen = binary.BigEndian.Uint16(data[p : p+2])
			p += 2
		}
		if len >= p+int(extraLen) {
			m.Extra = data[p : p+int(extraLen)]
			p += int(extraLen)
		} else {
			return errors.New("incorrect packet len for Extra")
		}
	}

	return nil
}

func (m *Message) Marshal() []byte {
	messageLength := 2 + 1 + 2 + 2 + 1 + 8 + 8 + 2 + len(m.Payload)
	if m.HasFlag(UdpMessageFlagDest) {
		messageLength += 8
	}

	if m.HasFlag(UdpMessageFlagExtra) {
		messageLength += 2 + len(m.Extra)
	}

	buf := make([]byte, messageLength)
	p := 0
	binary.BigEndian.PutUint16(buf[p:p+2], uint16(m.Tseq))
	p += 2
	buf[p] = m.Tid
	p += 1
	binary.BigEndian.PutUint16(buf[p:p+2], m.Timestamp)
	p += 2
	versionAndFlags := (m.Flags & 0x0fff) | (m.Version&0x000f)<<12
	binary.BigEndian.PutUint16(buf[p:p+2], uint16(versionAndFlags))
	p += 2
	buf[p] = m.MsgType
	p += 1
	binary.BigEndian.PutUint64(buf[p:p+8], m.From)
	p += 8
	binary.BigEndian.PutUint64(buf[p:p+8], m.To)
	p += 8
	if m.HasFlag(UdpMessageFlagDest) {
		binary.BigEndian.PutUint64(buf[p:p+8], m.Dest)
		p += 8
	}
	binary.BigEndian.PutUint16(buf[p:p+2], uint16(len(m.Payload)))
	p += 2
	copy(buf[p:p+int(len(m.Payload))], m.Payload)
	p += int(len(m.Payload))

	if m.HasFlag(UdpMessageFlagExtra) {
		binary.BigEndian.PutUint16(buf[p:p+2], uint16(len(m.Extra)))
		p += 2
		copy(buf[p:p+int(len(m.Extra))], m.Extra)
	}

	return buf
}

func (m *Message) SetFlag(flag uint16) {
	m.Flags = m.Flags | flag
}

func (m *Message) UnSetFlag(flag uint16) {
	m.Flags = m.Flags & (^flag)
}

func (m *Message) HasFlag(flag uint16) bool {
	return (m.Flags & flag) == flag
}

func (m *Message) NetTrafficSize() uint16 {
	size := 28 + 8 + 24 + len(m.Payload)
	if m.HasFlag(UdpMessageFlagDest) {
		size += 8
	}
	if m.HasFlag(UdpMessageFlagExtra) {
		size += 2 + len(m.Extra)
	}

	return uint16(size)
}


