/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/xujiajundd/ycng/utils/logging"
)

type Service struct {
	config          *Config
	sessions        map[uint64]*Session
	udp_server      *UdpServer
	tcp_server      *TcpServer
	packetReceiveCh chan *ReceivedPacket

	isRunning bool
	lock      sync.RWMutex
	stop      chan struct{}
	wg        sync.WaitGroup
}

func NewService(config *Config) *Service {
	service := &Service{
		config:          config,
		sessions:        make(map[uint64]*Session),
		packetReceiveCh: make(chan *ReceivedPacket, 10),
		isRunning:       false,
		stop:            make(chan struct{}),
	}

	service.udp_server = NewUdpServer(config, service.packetReceiveCh)

	return service
}

func (s *Service) Start() (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.isRunning {
		s.udp_server.Start()
		s.isRunning = true

		s.wg.Add(1)
		go s.loop()
	}
	return nil
}

func (s *Service) Stop() (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.isRunning {
		s.udp_server.Stop()
		s.isRunning = false
	}
	close(s.stop)
	return nil
}

func (s *Service) WaitForShutdown() {
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		if err := s.Stop(); err != nil {
		}
	}()

	s.wg.Wait()
	return
}

func (s *Service) loop() {

	defer s.wg.Done()

	for {
		select {
		case <-s.stop:
			return
		case packet := <-s.packetReceiveCh:
			s.handlePacket(packet)
		}
	}
}

func (s *Service) handlePacket(packet *ReceivedPacket) {
	//TODO：这个可以做性能优化，分配到多个线程去处理
	msg, err := NewMessageFromObfuscatedData(packet.body)
	if err != nil {
		logging.Logger.Warn("error:", err)
	}

	switch msg.msgType {
	case UdpMessageTypeNoop:
		s.handleMessageNoop(msg)

	case UdpMessageTypeTurnReg:
		s.handleMessageTurnReg(msg, packet)

	case UdpMessageTypeTurnRegReceived:

	case UdpMessageTypeAudioStream:
		s.handleMessageAudioStream(msg)

	case UdpMessageTypeVideoStream:
		s.handleMessageVideoStream(msg)

	case UdpMessageTypeVideoStreamIFrame:
		s.handleMessageVideoStreamIFrame(msg)

	case UdpMessageTypeVideoNack:
		s.handleMessageVideoNack(msg)

	case UdpMessageTypeVideoAskForIFrame:
	case UdpMessageTypeVideoOnlyIFrame:
	case UdpMessageTypeVideoOnlyAudio:
	case UdpMessageTypeClientReg:
	case UdpMessageTypeClientSignal:
	}
}

func (s *Service) handleMessageNoop(msg *Message) {
	logging.Logger.Info("received noop")
}

func (s *Service) handleMessageTurnReg(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received turn reg from ", msg.from, " for session ", msg.to)

	//检查当前session是否存在
	session := s.sessions[msg.to]
	if session == nil {
		session = NewSession(msg.to)
		session.Participants = make(map[uint64]*Participant)
		s.sessions[msg.to] = session
	}

	//当前用户注册到session
	participant := &Participant{Id: msg.from, UdpAddr: packet.fromUdpAddr, TcpConn: nil}
	session.Participants[participant.Id] = participant
}

func (s *Service) handleMessageAudioStream(msg *Message) {
	logging.Logger.Info("received audio from ", msg.from, " to ", msg.to)

	session := s.sessions[msg.to]
	if session != nil {
		for _, p := range session.Participants {
			if p.Id != msg.from {
				s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
			}
		}
	} else {
		logging.Logger.Info("session not existed for audio ", msg.to)
	}
}

func (s *Service) handleMessageVideoStream(msg *Message) {

}

func (s *Service) handleMessageVideoStreamIFrame(msg *Message) {

}

func (s *Service) handleMessageVideoNack(msg *Message) {

}
