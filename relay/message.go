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
	UdpMessageTypeAudioStream       = 20 //音频包
	UdpMessageTypeVideoStream       = 30 //视频包
	UdpMessageTypeVideoStreamIFrame = 31 //视频i帧
	UdpMessageTypeVideoNack         = 32 //视频请求重发包
	UdpMessageTypeVideoAskForIFrame = 33 //视频请求i帧
	UdpMessageTypeVideoOnlyIFrame   = 34 //视频只收i帧
	UdpMessageTypeVideoOnlyAudio    = 35 //视频只收音频

	UdpMessageTypeUserReg    = 200 //注册一个客户端
	UdpMessageTypeUserSignal = 201 //通过UDP来转发的信令，信令统一在push中定义
)

const (
	UdpMessageFlagExtra = 1 << 0
	UdpMessageFlagDest  = 1 << 1
)

type Message struct {
	tseq    int16
	tid     byte
	version uint16
	flags   uint16
	msgType uint8
	from    uint64
	to      uint64
	dest    uint64
	payload []byte
	extra []byte
}

type ReceivedPacket struct {
	fromUdpAddr *net.UDPAddr
	fromTcpConn *net.TCPConn
	body        []byte
	time        int64
}

func NewMessage(msgType uint8, from uint64, to uint64, dest uint64, payload []byte, extra []byte) *Message {
	msg := &Message{
		tseq:    0,
		tid:     0,
		version: 1,
		flags:   0,
		msgType: msgType,
		from:    from,
		to:      to,
		dest:    dest,
		payload: payload,
		extra:   extra,
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

	if len < 24 {
		return errors.New("incorrect packet, len < 24")
	}
	m.tseq = int16(binary.BigEndian.Uint16(data[p : p+2]))
	p += 2

	m.tid = data[p]
	p += 1

	versionAndFlags := binary.BigEndian.Uint16(data[p : p+2])
	p += 2
	m.version = (versionAndFlags & 0xf000) >> 12
	m.flags = versionAndFlags & 0xfff

	m.msgType = data[p]
	p += 1

	if len >= p+8 {
		m.from = binary.BigEndian.Uint64(data[p : p+8])
		p += 8
	}
	if len >= p+8 {
		m.to = binary.BigEndian.Uint64(data[p : p+8])
		p += 8
	}

	if m.HasFlag(UdpMessageFlagDest) {
		if len >= p+8 {
			m.dest = binary.BigEndian.Uint64(data[p : p+8])
			p += 8
		}
	}

	var payloadLen uint16
	if len >= p+2 {
		payloadLen = binary.BigEndian.Uint16(data[p : p+2])
		p += 2
	}

	if len >= p+int(payloadLen) {
		//TODO; 这个地方是copy呢？还是直接这样呢?
		m.payload = data[p : p+int(payloadLen)]
		p += int(payloadLen)
	}

	if m.HasFlag(UdpMessageFlagExtra) {
		var extraLen uint16
		if len >= p+2 {
			extraLen = binary.BigEndian.Uint16(data[p : p+2])
			p += 2
		}
		if len >= p+int(extraLen) {
			m.extra = data[p : p+int(extraLen)]
			p += int(extraLen)
		}
	}

	return nil
}

func (m *Message) Marshal() []byte {
	messageLength := 2 + 1 + 2 + 1 + 8 + 8 + 2 + len(m.payload)
	if m.HasFlag(UdpMessageFlagDest) {
		messageLength += 8
	}

	if m.HasFlag(UdpMessageFlagExtra) {
		messageLength += 2 + len(m.extra)
	}

	buf := make([]byte, messageLength)
	p := 0
	binary.BigEndian.PutUint16(buf[p:p+2], uint16(m.tseq))
	p += 2
	buf[p] = m.tid
	p += 1
	versionAndFlags := (m.flags & 0x0fff) | (m.version&0x000f)<<12
	binary.BigEndian.PutUint16(buf[p:p+2], uint16(versionAndFlags))
	p += 2
	buf[p] = m.msgType
	p += 1
	binary.BigEndian.PutUint64(buf[p:p+8], m.from)
	p += 8
	binary.BigEndian.PutUint64(buf[p:p+8], m.to)
	p += 8
	if m.HasFlag(UdpMessageFlagDest) {
		binary.BigEndian.PutUint64(buf[p:p+8], m.dest)
		p += 8
	}
	binary.BigEndian.PutUint16(buf[p:p+2], uint16(len(m.payload)))
	p += 2
	copy(buf[p:p+int(len(m.payload))], m.payload)
	p += int(len(m.payload))

	if m.HasFlag(UdpMessageFlagExtra) {
		binary.BigEndian.PutUint16(buf[p:p+2], uint16(len(m.extra)))
		p += 2
		copy(buf[p:p+int(len(m.extra))], m.extra)
	}

	return buf
}

func (m *Message) SetFlag(flag uint16) {
	m.flags = m.flags | flag
}

func (m *Message) HasFlag(flag uint16) bool {
	return (m.flags & flag) == flag
}

func (m *Message) Payload() []byte {
	return m.payload
}
