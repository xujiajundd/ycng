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
	"github.com/xujiajundd/ycng/utils/logging"
	"encoding/json"
	"github.com/xujiajundd/ycng/utils"
)

type SessionManager struct {
	sessions     map[uint64]*Session
	relays       []string
	saddr        string
	conn         *net.UDPConn
	subscriberCh chan *relay.ReceivedPacket
    dedup        *utils.LRU
	isRunning bool
	lock      sync.RWMutex
	stop      chan struct{}
	wg        sync.WaitGroup
	ticker    *time.Ticker
}

func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions:     make(map[uint64]*Session),
		saddr:        ":20005",
		subscriberCh: make(chan *relay.ReceivedPacket),
		dedup: utils.NewLRU(100, nil),
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
    sm.registerUserToRelays()  //每隔200秒重新注册一次
}

func (sm *SessionManager) handleMessageUserSignal(msg *relay.Message) {
	//去重
    if sm.dedup.Contains(string(msg.Payload)) {
    	    return
	} else {
		sm.dedup.Add(string(msg.Payload), true)
	}

    signal := NewSignal()
	json.Unmarshal(msg.Payload, signal)
	logging.Logger.Info(signal, signal.From, signal.To)

	msg.From = 0xffffffffffffffff
	msg.To = signal.To
	sm.sendMessageToRelays(msg)
}

func (sm *SessionManager) registerUserToRelays() {
	msg := relay.NewMessage(relay.UdpMessageTypeUserReg,
			0xffffffffffffffff, 0, 0, nil,nil)
    sm.sendMessageToRelays(msg)
}

func (sm *SessionManager) sendMessageToRelays(msg *relay.Message) {
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
