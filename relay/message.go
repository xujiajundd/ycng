/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import "net"

/*
relay需要解混淆和加混淆
relay需要的信息：是否i帧？
*/

const (
	UdpMessageTypeNoop              = 0   //NOOP
	UdpMessageTypeTurnReg           = 1   //客户端注册一个session的请求
	UdpMessageTypeTurnRegReceived   = 2   //服务器返回请求收到
	UdpMessageTypeAudioStream       = 20  //音频包
	UdpMessageTypeVideoStream       = 30  //视频包
	UdpMessageTypeVideoNack         = 31  //视频请求重发包
	UdpMessageTypeVideoAskForIFrame = 32  //视频请求i帧
	UdpMessageTypeVideoOnlyIFrame   = 33  //视频只收i帧
	UdpMessageTypeVideoRefuse       = 34  //视频只收音频
	UdpMessageTypeClientReg         = 100 //注册一个客户端
	UdpMessageTypeClientSignal      = 101 //通过UDP来转发的信令，信令统一在push中定义
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
	padding         []byte
}

type ReceivedPacket struct {
	fromUdpAddr *net.UDPAddr
	fromTcpConn *net.TCPConn
	body        []byte
}

func NewMessageFromObfuscatedData(data []byte) (message *Message, err error) {
	message := &Message{}


	return message, nil
}

func (m *Message) ObfuscatedDataOfMessage(message *Message) (data []byte, err error) {

}