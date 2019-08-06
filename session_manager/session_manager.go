/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package session_manager

import (
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/xujiajundd/ycng/relay"
	"github.com/xujiajundd/ycng/utils"
	"github.com/xujiajundd/ycng/utils/logging"
	"math/rand"
)

const (
	SessionManagerUserId = 0xffffffffffffffff
)

type SessionManager struct {
	sessions     map[uint64]*Session
	relays       []string
	saddr        string
	conn         *net.UDPConn
	subscriberCh chan *relay.ReceivedPacket
	dedup        *utils.LRU
	isRunning    bool
	lock         sync.RWMutex
	stop         chan struct{}
	wg           sync.WaitGroup
	ticker       *time.Ticker
}

func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions:     make(map[uint64]*Session),
		saddr:        ":20005",
		subscriberCh: make(chan *relay.ReceivedPacket),
		dedup:        utils.NewLRU(100, nil),
		isRunning:    false,
		stop:         make(chan struct{}),
		ticker:       time.NewTicker(200 * time.Second),
	}
	sm.GetRelays()
	return sm
}

func (sm *SessionManager) Start() {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if !sm.isRunning {
		sm.isRunning = true
		sm.wg.Add(1)
		addr, err := net.ResolveUDPAddr("udp4", sm.saddr)
		if err != nil {
			logging.Logger.Error("error ResolveUDPAddr")
		}

		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			logging.Logger.Error("error ListenUDP", err)
			return
		}
		logging.Logger.Info("listen on port:", sm.saddr)

		sm.conn = conn

		sm.registerUserToRelays()

		go sm.loop()
		go sm.handleClient()
	}
}

func (sm *SessionManager) Stop() {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if sm.isRunning {
		sm.isRunning = false
	}
	close(sm.stop)
}

func (sm *SessionManager) WaitForShutdown() {
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		sm.Stop()
	}()

	sm.wg.Wait()
	return
}

func (sm *SessionManager) loop() {
	defer sm.wg.Done()

	for {
		select {
		case <-sm.stop:
			return
		case packet := <-sm.subscriberCh:
			sm.handlePacket(packet)
		case time := <-sm.ticker.C:
			sm.handleTicker(time)
		}
	}
}

func (sm *SessionManager) handleClient() {
	var buf [2048]byte

	for {
		size, addr, err := sm.conn.ReadFromUDP(buf[0:])
		if err != nil {
			logging.Logger.Error("error ReadFromUDP ", err)
			continue
		}

		data := make([]byte, size)
		copy(data, buf[0:size])
		packet := &relay.ReceivedPacket{
			Body:        data,
			FromUdpAddr: addr,
			Time:        time.Now().UnixNano(),
		}

		sm.subscriberCh <- packet
	}
}

func (sm *SessionManager) handlePacket(packet *relay.ReceivedPacket) {
	msg, err := relay.NewMessageFromObfuscatedData(packet.Body)
	if err != nil {
		logging.Logger.Warn("error:", err)
		return
	}

	switch msg.MsgType {
	case relay.UdpMessageTypeUserRegReceived:
		logging.Logger.Info("user reg received from ", packet.FromUdpAddr)
	case relay.UdpMessageTypeUserSignal:
		sm.handleMessageUserSignal(msg)
	default:
		logging.Logger.Warn("unrecognized message type")
	}
}

func (sm *SessionManager) handleTicker(now time.Time) {
	//每隔200秒重新注册一次
	sm.registerUserToRelays()

	//清理已经结束的session，1-1有收到过end，多方发出或收到所有的end。或者sm主动轮询参与者？
}

func (sm *SessionManager) handleMessageUserSignal(msg *relay.Message) {
	//去重
	if sm.dedup.Contains(string(msg.Payload)) {
		return
	} else {
		sm.dedup.Add(string(msg.Payload), true)
	}

	signal := NewSignalTemp()
	err := signal.Unmarshal(msg.Payload)
	if err != nil {
		logging.Logger.Warn("signal unmarshal error:", err)
		return
	}

	//1.如果invite中sid=0，则回复serverring带一个sid
	//2.如果session不存在，创建一个session
	//3.如果1-1mode，透明转发信令
	//4.如果是多方模式，作为呼叫的一方参与，然后发member_state给所有参与方
	//5.如果1-1mode时，收到member_op，转为多方模式。
	//6.signal中的to为session_manager则为多方，如果to是其他帐号，则是1-1

	if signal.Signal == YCKCallSignalTypeSidRequest {
		//生成一个与现存不重复的sid
		var sid uint64
		for {
			sid = rand.Uint64()
			if sm.sessions[sid] == nil {
				break
			}
		}
		//创建session
		session := NewSession(sid)
		sm.sessions[sid] = session

		//回复信令
		sid_created := NewSignal(YCKCallSignalTypeSidCreated, SessionManagerUserId, signal.From, sid)
		payload, err := sid_created.Marshal()
		if err == nil {
			msg := relay.NewMessage(relay.UdpMessageTypeUserSignal, SessionManagerUserId, signal.From, 0, payload, nil)
			sm.sendSignalMessage(msg)
		} else {
			logging.Logger.Warn("signal marshal error:", err)
		}
		return
	}

	if signal.SessionId == 0 {
		logging.Logger.Warn("error signal:", signal.Signal, " with sid=0 ", signal.From, signal.To)
		//return
	}

	session := sm.sessions[signal.SessionId]
	if session == nil {
		logging.Logger.Warn("session not existed for ", signal)
		//return
	}


	if signal.To != SessionManagerUserId {
		//转发signal
		if session.Mode == YCKCallModeMultiple { //进入多方模式后，不能再接受1-1信令
			logging.Logger.Warn("receive 1-1 signal when in multipart mode")
			return
		}
		payload, err := signal.Marshal()
		if err == nil {
			msg := relay.NewMessage(relay.UdpMessageTypeUserSignal, SessionManagerUserId, signal.To, 0, payload, nil)
			sm.sendSignalMessage(msg)
		} else {
			logging.Logger.Warn("signal marshal error:", err)
		}
	} else {
		//管理session，member状态
		switch signal.Signal {
		case YCKCallSignalTypeInvite:

		case YCKCallSignalTypeCancel:

		case YCKCallSignalTypeEnd:

		case YCKCallSignalTypeMemberOp:
			if session.Mode == YCKCallModeOneToOne { //1-1模式时收到多方信令则转入多方模式，并且要通知所有参与方改模式
				session.Mode = YCKCallModeMultiple
			}
		default:

		}
	}
}

func (sm *SessionManager) registerUserToRelays() {
	msg := relay.NewMessage(relay.UdpMessageTypeUserReg,
		SessionManagerUserId, 0, 0, nil, nil)
	sm.sendSignalMessageByRelays(msg)
}

func (sm *SessionManager) sendSignalMessageByRelays(msg *relay.Message) {
	data := msg.ObfuscatedDataOfMessage()

	for _, relay := range sm.relays {
		udpAddr, err := net.ResolveUDPAddr("udp4", relay)
		if err != nil {
			logging.Logger.Error("incorrect addr ", err)
		}

		_, err = sm.conn.WriteToUDP(data, udpAddr)
		if err != nil {
			logging.Logger.Error("udp write error", err)
		}
	}
}

func (sm *SessionManager) sendSignalMessage(msg *relay.Message) {
	sm.sendSignalMessageByRelays(msg)
}

func (sm *SessionManager) GetRelays() {
	sm.relays = []string{
		"10.18.96.46:19001",
		"10.18.96.46:19002",
		"10.18.96.46:19003",
		"106.75.106.193:19001",
		"117.50.61.49:19001",
		"117.50.63.224:19001",
	}
}
