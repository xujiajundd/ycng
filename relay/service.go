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
	"time"
)

type Service struct {
	config          *Config
	sessions        map[uint64]*Session
	users           map[uint64]*User
	storage         *Storage
	udp_server      *UdpServer
	tcp_server      *TcpServer
	packetReceiveCh chan *ReceivedPacket //通过udp或者tcp进来的包

	isRunning bool
	lock      sync.RWMutex
	stop      chan struct{}
	wg        sync.WaitGroup
	ticker    *time.Ticker
}

func NewService(config *Config) *Service {
	service := &Service{
		config:          config,
		sessions:        make(map[uint64]*Session),
		users:           make(map[uint64]*User),
		storage:         NewStorage(),
		packetReceiveCh: make(chan *ReceivedPacket, 10),
		isRunning:       false,
		stop:            make(chan struct{}),
		ticker:          time.NewTicker(1 * time.Second),
	}

	service.udp_server = NewUdpServer(config, service.packetReceiveCh)
	service.tcp_server = NewTcpServer(config, service.packetReceiveCh)

	return service
}

func (s *Service) Start() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.isRunning {
		s.udp_server.Start()
		s.tcp_server.Start()
		s.isRunning = true

		s.wg.Add(1)
		go s.loop()
	}
}

func (s *Service) Stop() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.isRunning {
		s.udp_server.Stop()
		s.tcp_server.Stop()
		s.isRunning = false
	}
	close(s.stop)
}

func (s *Service) WaitForShutdown() {
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		s.Stop()
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
		case time := <-s.ticker.C:
           s.handleTicker(time)
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

	case UdpMessageTypeTurnRegReceived: //只有客户端会收到这个

	case UdpMessageTypeAudioStream:
		s.handleMessageAudioStream(msg)

	case UdpMessageTypeVideoStream:
		s.handleMessageVideoStream(msg)

	case UdpMessageTypeVideoStreamIFrame:
		s.handleMessageVideoStreamIFrame(msg)

	case UdpMessageTypeVideoNack:
		s.handleMessageVideoNack(msg)

	case UdpMessageTypeVideoAskForIFrame:
		s.handleMessageVideoASkForIFrame(msg)

	case UdpMessageTypeVideoOnlyIFrame:

	case UdpMessageTypeVideoOnlyAudio:

	case UdpMessageTypeUserReg:
		s.handleMessageUserReg(msg, packet)

	case UdpMessageTypeUserSignal:
		s.handleMessageUserSignal(msg)

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

	//回复
	msg.msgType = UdpMessageTypeTurnRegReceived
	s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), participant.UdpAddr)
}

func (s *Service) handleMessageAudioStream(msg *Message) {
	//logging.Logger.Info("received audio from ", msg.from, " to ", msg.to)

	session := s.sessions[msg.to]
	if session != nil {
		for _, p := range session.Participants {
			if p.Id != msg.from || p.Id == 0 {
				s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
			}
		}
	} else {
		logging.Logger.Info("session not existed", msg.to)
	}
}

func (s *Service) handleMessageVideoStream(msg *Message) {
	//logging.Logger.Info("received video from ", msg.from, " to ", msg.to)

	session := s.sessions[msg.to]
	if session != nil {
		for _, p := range session.Participants {
			if p.Id != msg.from || p.Id == 0 {
				s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
			}
		}
	} else {
		logging.Logger.Info("session not existed", msg.to)
	}
}

func (s *Service) handleMessageVideoStreamIFrame(msg *Message) {
	logging.Logger.Info("received video iframe from ", msg.from, " to ", msg.to)

	session := s.sessions[msg.to]
	if session != nil {
		for _, p := range session.Participants {
			if p.Id != msg.from || p.Id == 0 {
				s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
			}
		}
	} else {
		logging.Logger.Info("session not existed", msg.to)
	}
}

func (s *Service) handleMessageVideoASkForIFrame(msg *Message) {
	logging.Logger.Info("received ask for iframe from ", msg.from, " to ", msg.to)

	session := s.sessions[msg.to]
	if session != nil {
		for _, p := range session.Participants {
			if p.Id != msg.from || p.Id == 0 {
				s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
			}
		}
	} else {
		logging.Logger.Info("session not existed ", msg.to)
	}
}

func (s *Service) handleMessageVideoNack(msg *Message) {
	logging.Logger.Info("received nack from ", msg.from, " to ", msg.to)

	session := s.sessions[msg.to]
	if session != nil {
		for _, p := range session.Participants {
			if p.Id != msg.from || p.Id == 0 {
				s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
			}
		}
	} else {
		logging.Logger.Info("session not existed ", msg.to)
	}
}

func (s *Service) handleMessageUserReg(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received user reg from ", msg.from, " to ", msg.to)

    user := s.users[msg.from]
    if user == nil {
    	user := NewUser(msg.from)
    	s.users[msg.from] = user
	}

	user.UdpAddr = packet.fromUdpAddr
	user.LastActiveTime = time.Now()
}

func (s *Service) handleMessageUserSignal(msg *Message) {
	logging.Logger.Info("received user signal from ", msg.from, " to ", msg.to)

	user := s.users[msg.to]

	if user != nil {
		s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), user.UdpAddr)
	}
}


//清理过期的session和user
func (s *Service) handleTicker(time time.Time) {

}