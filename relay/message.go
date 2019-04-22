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

	UdpMessageTypeClientReg    = 100 //注册一个客户端
	UdpMessageTypeClientSignal = 101 //通过UDP来转发的信令，信令统一在push中定义
)

type Message struct {
	versionAndFlags uint16
	msgType         uint8
	from            uint64
	to              uint64
	payloadLen      uint16
	payload         []byte
	extraLen        uint16
	extra           []byte
}

type ReceivedPacket struct {
	fromUdpAddr *net.UDPAddr
	fromTcpConn *net.TCPConn
	body        []byte
}

func NewMessage(vf uint16, msgType uint8, from uint64, to uint64, plen uint16, payload []byte, elen uint16, extra []byte) *Message {
	msg := &Message{
		versionAndFlags: vf,
		msgType:         msgType,
		from:            from,
		to:              to,
		payloadLen:      plen,
		payload:         payload,
		extraLen:        elen,
		extra:           extra,
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

	if len < 3 {
		return errors.New("incorrect packet, len < 3")
	}

	m.versionAndFlags = binary.BigEndian.Uint16(data[p : p+2])
	p += 2
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
	if len >= p+2 {
		m.payloadLen = binary.BigEndian.Uint16(data[p : p+2])
		p += 2
	}
	if len >= p+int(m.payloadLen) {
		//TODO; 这个地方是copy呢？还是直接这样呢?
		m.payload = data[p : p+int(m.payloadLen)]
		p += int(m.payloadLen)
	}
	if len >= p+2 {
		m.extraLen = binary.BigEndian.Uint16(data[p : p+2])
		p += 2
	}
	if len >= p+int(m.extraLen) {
		m.extra = data[p : p+int(m.extraLen)]
		p += int(m.extraLen)
	}

	return nil
}

func (m *Message) Marshal() []byte {
	messageLength := 2 + 1 + 8 + 8 + 2 + 2 + len(m.payload) + len(m.extra)
	buf := make([]byte, messageLength)
	p := 0
	binary.BigEndian.PutUint16(buf[p:p+2], m.versionAndFlags)
	p += 2
	buf[p] = m.msgType
	p += 1
	binary.BigEndian.PutUint64(buf[p:p+8], m.from)
	p += 8
	binary.BigEndian.PutUint64(buf[p:p+8], m.to)
	p += 8
	binary.BigEndian.PutUint16(buf[p:p+2], m.payloadLen)
	p += 2
	copy(buf[p:p+int(m.payloadLen)], m.payload)
	p += int(m.payloadLen)
	binary.BigEndian.PutUint16(buf[p:p+2], m.extraLen)
	p += 2
	copy(buf[p:p+int(m.extraLen)], m.extra)

	return buf
}

func (m *Message) Payload() []byte {
	return m.payload
}
