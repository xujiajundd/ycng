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
		ticker:          time.NewTicker(10 * time.Second),
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
	msg, err := NewMessageFromObfuscatedData(packet.Body)
	if err != nil {
		logging.Logger.Warn("error:", err)
		return
	}

	switch msg.MsgType {
	case UdpMessageTypeNoop:
		s.handleMessageNoop(msg)

	case UdpMessageTypeTurnReg:
		s.handleMessageTurnReg(msg, packet)

	case UdpMessageTypeTurnRegReceived: //只有客户端会收到这个

	case UdpMessageTypeAudioStream:
		s.handleMessageAudioStream(msg, packet)

	case UdpMessageTypeVideoStream:
		s.handleMessageVideoStream(msg, packet)

	case UdpMessageTypeVideoStreamIFrame:
		s.handleMessageVideoStreamIFrame(msg, packet)

	case UdpMessageTypeVideoNack:
		s.handleMessageVideoNack(msg, packet)

	case UdpMessageTypeVideoAskForIFrame:
		s.handleMessageVideoASkForIFrame(msg, packet)

	case UdpMessageTypeVideoOnlyIFrame:

	case UdpMessageTypeVideoOnlyAudio:

	case UdpMessageTypeUserReg:
		s.handleMessageUserReg(msg, packet)

	case UdpMessageTypeUserSignal:
		s.handleMessageUserSignal(msg)
	default:
		logging.Logger.Warn("unrecognized message type")
	}
}

func (s *Service) handleMessageNoop(msg *Message) {
	logging.Logger.Info("received noop")
}

func (s *Service) handleMessageTurnReg(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received turn reg From ", msg.From, " for session ", msg.To)

	//检查当前session是否存在
	session := s.sessions[msg.To]
	if session == nil {
		session = NewSession(msg.To)
		session.Participants = make(map[uint64]*Participant)
		s.sessions[msg.To] = session
	}

	//当前用户注册到session
	participant := &Participant{Id: msg.From, UdpAddr: packet.FromUdpAddr, TcpConn: nil}
	participant.LastActiveTime = time.Now()
	participant.Metrics = NewMetrics()
	session.Participants[participant.Id] = participant

	//回复
	msg.MsgType = UdpMessageTypeTurnRegReceived
	s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), participant.UdpAddr)

	//bugfix：将其他participant的pengingMsg检查一遍，如果有上次遗留的则清除。
	for _, p := range session.Participants {
		if p.PendingMsg != nil {
			if p.PendingMsg.From == msg.From {
				p.PendingMsg = nil
			}
		}
	}
}

func (s *Service) handleMessageAudioStream(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received audio From ", msg.From, " To ", msg.To)

	session := s.sessions[msg.To]
	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			participant.Metrics.AddEntry(msg.Tid, msg.Tseq, msg.NetTrafficSize())
			for _, p := range session.Participants {
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) { //后一个条件是为了本地回环测试，非登录用户的id为0
					if p.PendingMsg == nil {
						p.PendingMsg = msg
					} else {
						p.PendingMsg.Tseq = p.Tseq
						msg.Tseq = p.Tseq
						p.Tseq++
						s.udp_server.SendPacket(p.PendingMsg.ObfuscatedDataOfMessage(), p.UdpAddr)
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						p.PendingMsg = nil
					}
				}
			}
		} else {
			logging.Logger.Info("participant", msg.From, " not existed in session", msg.To)
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session not existed", msg.To)
		s.askForReTurnReg(msg, packet)
	}
}

func (s *Service) handleMessageVideoStream(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received video From ", msg.From, " To ", msg.To)

	session := s.sessions[msg.To]
	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			participant.Metrics.AddEntry(msg.Tid, msg.Tseq, msg.NetTrafficSize())
			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) { //后一个条件是为了本地回环测试，非登录用户的id为0
					if p.PendingMsg == nil {
						p.PendingMsg = msg
					} else {
						p.PendingMsg.Tseq = p.Tseq
						msg.Tseq = p.Tseq
						p.Tseq++
						s.udp_server.SendPacket(p.PendingMsg.ObfuscatedDataOfMessage(), p.UdpAddr)
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						p.PendingMsg = nil
					}
				}
			}
		} else {
			logging.Logger.Info("participant", msg.From, " not existed in session", msg.To)
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session not existed", msg.To)
		s.askForReTurnReg(msg, packet)
	}
}

func (s *Service) handleMessageVideoStreamIFrame(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received video iframe From ", msg.From, " To ", msg.To)

	session := s.sessions[msg.To]
	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			participant.Metrics.AddEntry(msg.Tid, msg.Tseq, msg.NetTrafficSize())
			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) { //后一个条件是为了本地回环测试，非登录用户的id为0
					if p.PendingMsg == nil {
						p.PendingMsg = msg
					} else {
						p.PendingMsg.Tseq = p.Tseq
						msg.Tseq = p.Tseq
						p.Tseq++
						s.udp_server.SendPacket(p.PendingMsg.ObfuscatedDataOfMessage(), p.UdpAddr)
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						p.PendingMsg = nil
					}
				}
			}
		} else {
			logging.Logger.Info("participant", msg.From, " not existed in session", msg.To)
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session not existed", msg.To)
		s.askForReTurnReg(msg, packet)
	}
}

func (s *Service) handleMessageVideoASkForIFrame(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received ask for iframe From ", msg.From, " To ", msg.To, " Dest ", msg.Dest)

	session := s.sessions[msg.To]

	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) {
					s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
				}
			}
		} else {
			logging.Logger.Info("participant", msg.From, " not existed in session", msg.To)
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session not existed ", msg.To)
		s.askForReTurnReg(msg, packet)
	}
}

func (s *Service) handleMessageVideoNack(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received nack From ", msg.From, " To ", msg.To, " Dest ", msg.Dest)

	session := s.sessions[msg.To]

	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) {
					s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
				}
			}
		} else {
			logging.Logger.Info("participant", msg.From, " not existed in session", msg.To)
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session not existed ", msg.To)
		s.askForReTurnReg(msg, packet)
	}
}

func(s *Service)askForReTurnReg(msg *Message, packet *ReceivedPacket){
	newMsg := NewMessage(UdpMessageTypeTurnRegNoExist, msg.From, msg.To, msg.Dest, nil, nil)
	newMsg.Tid = msg.Tid
	s.udp_server.SendPacket(newMsg.ObfuscatedDataOfMessage(), packet.FromUdpAddr)
}

func (s *Service) handleMessageUserReg(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received user reg From ", msg.From, " To ", msg.To, packet.FromUdpAddr)

	user := s.users[msg.From]
	if user == nil {
		user = NewUser(msg.From)
		s.users[msg.From] = user
	}

	user.UdpAddr = packet.FromUdpAddr
	user.LastActiveTime = time.Now()
	msg.MsgType = UdpMessageTypeUserRegReceived
	s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), user.UdpAddr)
}

func (s *Service) handleMessageUserSignal(msg *Message) {
	logging.Logger.Info("received user signal From ", msg.From, " To ", msg.To)

	user := s.users[msg.To]

	if user != nil {
		logging.Logger.Info("route user signal From ", msg.From, " To ", msg.To, user.UdpAddr)
		s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), user.UdpAddr)
	}
}

//清理过期的session和user
func (s *Service) handleTicker(now time.Time) {
	for skey, session := range s.sessions {
		for pkey, participant := range session.Participants {
			if now.Sub(participant.LastActiveTime) > 60*time.Second {
				delete(session.Participants, pkey)
				logging.Logger.Info("delete participant ", pkey, " From session ", skey, " for inactive 60s")
			}
		}
		if len(session.Participants) == 0 {
			delete(s.sessions, skey)
			logging.Logger.Info("delete session ", skey, " for all participants quit")
		}
	}

	for ukey, user := range s.users {
		if now.Sub(user.LastActiveTime) > 600*time.Second {
			delete(s.users, ukey)
			logging.Logger.Info("delete user ", ukey, " for inactive 10 minutes")
		}
	}

	if len(s.sessions) > 0 || len(s.users) > 0{
		logging.Logger.Infoln("details:")
		for skey, session := range s.sessions {
			logging.Logger.Info("    session: ", skey)
			for pkey, participant := range session.Participants {
				logging.Logger.Info("       participant:", pkey)
				participant.Metrics.Process() //这个临时放在这里，正常情况要放在反馈的地方。
			}
		}

		for ukey, _ := range s.users {
			logging.Logger.Info("    reg user:", ukey)
		}
	}
}
