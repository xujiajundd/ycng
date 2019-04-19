/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import "net"

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
type Participant struct {
	Id  []byte  //8 byte participant account id
	UdpAddr *net.UDPAddr  //当前udp地址
	TcpConn *net.TCPConn  //当前tcp连接
}

type Session struct {
	Id       []byte
	IdShort  []byte
	Type     int
	Participants map[string]*Participant
}

