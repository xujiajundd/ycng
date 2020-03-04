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

	"time"

	"github.com/xujiajundd/ycng/utils/logging"
	//"github.com/xujiajundd/ycng/utils"
	"bytes"
	"encoding/json"
)

type Service struct {
	config          *Config
	sessions        map[int64]*Session
	users           map[int64]*User
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
		sessions:        make(map[int64]*Session),
		users:           make(map[int64]*User),
		storage:         NewStorage(),
		packetReceiveCh: make(chan *ReceivedPacket, 10),
		isRunning:       false,
		stop:            make(chan struct{}),
		ticker:          time.NewTicker(60 * time.Second),
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
	//其实单线程也可以，如果server的资源有富余，可以起多个relay实例。
	msg, err := NewMessageFromObfuscatedData(packet.Body)
	if err != nil {
		logging.Logger.Warn("error:", err)
		return
	}

	switch msg.MsgType {
	case UdpMessageTypeNoop:
		s.handleMessageNoop(msg, packet)

	case UdpMessageTypeTurnReg:
		s.handleMessageTurnReg(msg, packet)

	case UdpMessageTypeTurnRegReceived: //只有客户端会收到这个

	case UdpMessageTypeTurnUnReg:
		s.handleMessageTurnUnReg(msg, packet)

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
		s.handleMessageVideoOnlyAudio(msg)

	case UdpMessageTypeUserReg:
		s.handleMessageUserReg(msg, packet)

	case UdpMessageTypeUserSignal:
		s.handleMessageUserSignal(msg, packet)
	default:
		logging.Logger.Warn("unrecognized message type")
	}
}

func (s *Service) handleMessageNoop(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received noop"), 收到noop，原样回复, 这个目前只在rtt测试的时候用到
	s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), packet.FromUdpAddr)
}

func (s *Service) handleMessageTurnReg(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received turn reg From ", msg.From, "<", packet.FromUdpAddr.String(), ">", " for session ", msg.To)

	//检查当前session是否存在
	session := s.sessions[msg.To]
	if session == nil {
		session = NewSession(msg.To)
		session.Participants = make(map[int64]*Participant)
		s.sessions[msg.To] = session
	}

	//当前用户注册到session
	participant := session.Participants[msg.From]
	if participant == nil {
		participant = &Participant{Id: msg.From, UdpAddr: packet.FromUdpAddr, TcpConn: nil}
		participant.Metrics = NewMetrics()
		participant.VideoQueueOut = NewQueueOut()
		participant.OnlyAcceptAudio = false
		session.Participants[participant.Id] = participant
	}
	participant.UdpAddr = packet.FromUdpAddr
	participant.LastActiveTime = time.Now()

	//回复
	msg.MsgType = UdpMessageTypeTurnRegReceived
	s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), participant.UdpAddr)

	//Turn info支持P2P隧道
	if len(session.Participants) == 2 {
		msg.MsgType = UdpMessageTypeTurnInfo
		//如果udp addr有变化，则向双发发布各自的外网地址
		//todo
		type PInfo struct {
			Id  int64
			Udp string
		}
		turnInfo := make([]PInfo, 0)
		for _, p := range session.Participants {
			ti := PInfo{Id: p.Id, Udp: p.UdpAddr.String()}
			turnInfo = append(turnInfo, ti)
		}

		data, err := json.Marshal(turnInfo)

		if err != nil {
			logging.Logger.Warn("turn info err", err)
		} else {
			msg.Payload = data
			packet := msg.ObfuscatedDataOfMessage()
			for _, p := range session.Participants {
				s.udp_server.SendPacket(packet, p.UdpAddr)
			}
		}
	}

	//bugfix：将其他participant的pengingMsg检查一遍，如果有上次遗留的则清除。
	for _, p := range session.Participants {
		if p.PendingMsg != nil {
			if p.PendingMsg.From == msg.From {
				p.PendingMsg = nil
			}
		}
	}
}

func (s *Service) handleMessageTurnUnReg(msg *Message, packet *ReceivedPacket) {
	//客户端退出是应该发这个消息，注销在Relay上的注册
	logging.Logger.Info("received turn unreg From ", msg.From, " for session ", msg.To)

	//检查当前session是否存在, 如已不存在，无须UnReg
	session := s.sessions[msg.To]
	if session == nil {
		return
	}

	//当前用户取消注册
	participant := session.Participants[msg.From]
	if participant == nil {
		return
	}
	delete(session.Participants, participant.Id)

	////如果剩下的参与方只有两个，也尝试发TurnInfo？
	//if len(session.Participants) == 2 {
	//	//todo
	//}
}

func (s *Service) handleMessageAudioStream(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received audio From ", msg.From, " To ", msg.To)

	session := s.sessions[msg.To]
	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			//如果客户端的外网地址有变化了，要更新。
			if !bytes.Equal(participant.UdpAddr.IP, packet.FromUdpAddr.IP) || participant.UdpAddr.Port != packet.FromUdpAddr.Port {
				logging.Logger.Warn("received packet from participant ", msg.From, " with changed udp address:", packet.FromUdpAddr.String(), " origin:", participant.UdpAddr.String())
				participant.UdpAddr = packet.FromUdpAddr
			}

			ok, data := participant.Metrics.Process(msg, packet.Time)
			if ok {
				participant.PendingExtra = data
			}
			for _, p := range session.Participants {
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) { //后一个条件是为了本地回环测试，非登录用户的id为0
					if p.PendingMsg == nil {
						p.PendingMsg = msg
					} else {
						p.PendingMsg.Tseq = p.Tseq
						msg.Tseq = p.Tseq
						p.Tseq++
						extraAdded := false
						if p.PendingExtra != nil && msg.Extra == nil {
							now := time.Now()
							delay := now.Sub(p.LastActiveTime) / time.Millisecond
							if delay < 250 {
								p.PendingExtra.Rdelay = uint8(delay)
								msg.Extra = p.PendingExtra.Marshal()
								msg.SetFlag(UdpMessageFlagExtra)
								p.PendingExtra = nil
								extraAdded = true
							} else {
								p.PendingExtra = nil
							}
						}
						s.udp_server.SendPacket(p.PendingMsg.ObfuscatedDataOfMessage(), p.UdpAddr)
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						if extraAdded {
							msg.Extra = nil
							msg.UnSetFlag(UdpMessageFlagExtra)
						}
						p.PendingMsg = nil
					}
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

func (s *Service) handleMessageVideoStream(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received video From ", msg.From, " To ", msg.To)

	session := s.sessions[msg.To]
	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			ok, data := participant.Metrics.Process(msg, packet.Time)
			if ok {
				participant.PendingExtra = data
			}
			participant.VideoQueueOut.AddItem(false, msg.Payload)
			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.OnlyAcceptAudio {
					continue
				}
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) { //后一个条件是为了本地回环测试，非登录用户的id为0
					if p.PendingMsg == nil {
						p.PendingMsg = msg
					} else {
						p.PendingMsg.Tseq = p.Tseq
						msg.Tseq = p.Tseq
						p.Tseq++
						extraAdded := false
						if p.PendingExtra != nil && msg.Extra == nil {
							now := time.Now()
							delay := now.Sub(p.LastActiveTime) / time.Millisecond
							if delay < 250 {
								p.PendingExtra.Rdelay = uint8(delay)
								msg.Extra = p.PendingExtra.Marshal()
								msg.SetFlag(UdpMessageFlagExtra)
								p.PendingExtra = nil
								extraAdded = true
							} else {
								p.PendingExtra = nil
							}
						}
						s.udp_server.SendPacket(p.PendingMsg.ObfuscatedDataOfMessage(), p.UdpAddr)
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						if extraAdded {
							msg.Extra = nil
							msg.UnSetFlag(UdpMessageFlagExtra)
						}
						p.PendingMsg = nil
					}
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

func (s *Service) handleMessageVideoStreamIFrame(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received video iframe From ", msg.From, " To ", msg.To)

	session := s.sessions[msg.To]
	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			participant.LastActiveTime = time.Now()
			ok, data := participant.Metrics.Process(msg, packet.Time)
			if ok {
				participant.PendingExtra = data
			}
			participant.VideoQueueOut.AddItem(true, msg.Payload)
			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.OnlyAcceptAudio {
					continue
				}
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) { //后一个条件是为了本地回环测试，非登录用户的id为0
					if p.PendingMsg == nil {
						p.PendingMsg = msg
					} else {
						p.PendingMsg.Tseq = p.Tseq
						msg.Tseq = p.Tseq
						p.Tseq++
						extraAdded := false
						if p.PendingExtra != nil && msg.Extra == nil {
							now := time.Now()
							delay := now.Sub(p.LastActiveTime) / time.Millisecond
							if delay < 250 {
								p.PendingExtra.Rdelay = uint8(delay)
								msg.Extra = p.PendingExtra.Marshal()
								msg.SetFlag(UdpMessageFlagExtra)
								p.PendingExtra = nil
								extraAdded = true
							} else {
								p.PendingExtra = nil
							}
						}
						s.udp_server.SendPacket(p.PendingMsg.ObfuscatedDataOfMessage(), p.UdpAddr)
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						if extraAdded {
							msg.Extra = nil
							msg.UnSetFlag(UdpMessageFlagExtra)
						}
						p.PendingMsg = nil
					}
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

func (s *Service) handleMessageVideoASkForIFrame(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received ask for iframe From ", msg.From, " To ", msg.To, " Dest ", msg.Dest)

	session := s.sessions[msg.To]

	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			//participant.LastActiveTime = time.Now()
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
	//logging.Logger.Info("received nack From ", msg.From, " To ", msg.To, " Dest ", msg.Dest)

	session := s.sessions[msg.To]

	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			//participant.LastActiveTime = time.Now()
			//解nack包
			nack := msg.Payload
			dest := session.Participants[msg.Dest]
			if dest == nil {
				return
			}
			_, n_tries, isIFrame, packets := dest.VideoQueueOut.ProcessNack(nack)
			//logging.Logger.Info("process nack from ", msg.From, " to sid ", msg.To, " dest ", msg.Dest, " seq ", seqid, " n_tries ", n_tries, " packets ", len(packets))

			//从Dest的QueueOut中查找是否可以响应nack
			if packets != nil && len(packets) > 0 {
				for i := 0; i < len(packets); i++ {
					packet := packets[i]
					nmsgType := UdpMessageTypeVideoStream
					if isIFrame {
						nmsgType = UdpMessageTypeVideoStreamIFrame
					}
					nmsg := NewMessage(uint8(nmsgType), msg.Dest, session.Id, msg.From, packet, nil)
					nmsg.Tid = msg.Tid
					if participant.PendingMsg == nil {
						participant.PendingMsg = nmsg
					} else {
						participant.PendingMsg.Tseq = participant.Tseq
						nmsg.Tseq = participant.Tseq
						participant.Tseq++
						s.udp_server.SendPacket(participant.PendingMsg.ObfuscatedDataOfMessage(), participant.UdpAddr)
						s.udp_server.SendPacket(nmsg.ObfuscatedDataOfMessage(), participant.UdpAddr)
						participant.PendingMsg = nil
					}
				}
			}

			//如果是tries>0且QueueOut中无响应，则发给Dest处理
			if n_tries > 1 && len(packets) == 0 {
				for _, p := range session.Participants {
					if msg.Dest != 0 && p.Id != msg.Dest {
						continue
					}
					if p.Id != msg.From || (p.Id == 0 && msg.From == 0) {
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
					}
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

func (s *Service) askForReTurnReg(msg *Message, packet *ReceivedPacket) {
	newMsg := NewMessage(UdpMessageTypeTurnRegNoExist, msg.From, msg.To, msg.Dest, nil, nil)
	newMsg.Tid = msg.Tid
	s.udp_server.SendPacket(newMsg.ObfuscatedDataOfMessage(), packet.FromUdpAddr)
}

func (s *Service) handleMessageVideoOnlyAudio(msg *Message) {
	session := s.sessions[msg.To]

	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			payload := msg.Payload
			if len(payload) == 1 {
				stopFlag := uint8(payload[0])
				if stopFlag == 0 {
					participant.OnlyAcceptAudio = false
				} else {
					participant.OnlyAcceptAudio = true
				}
			} else {
				logging.Logger.Info("participant", msg.From, " incorrect audio only request")
			}
		} else {
			logging.Logger.Info("participant", msg.From, " not existed in session", msg.To)
		}
	} else {
		logging.Logger.Info("session not existed ", msg.To)
	}
}

func (s *Service) handleMessageUserReg(msg *Message, packet *ReceivedPacket) {
	logging.Logger.Info("received user reg From ", msg.From, "<", packet.FromUdpAddr.String(), ">", " To ", msg.To)

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

func (s *Service) handleMessageUserSignal(msg *Message, packet *ReceivedPacket) {
	signal := NewSignalTemp()
	err := signal.Unmarshal(msg.Payload)
	if err != nil {
		logging.Logger.Warn("signal unmarshal error:", err)
	}

	//State sync和state info两个信令太多，不打在日志之中了。
	if signal.Signal != YCKCallSignalTypeStateSync && signal.Signal != YCKCallSignalTypeStateInfo {
		logging.Logger.Info("received user signal From ", msg.From, "<", packet.FromUdpAddr.String(), ">", " To ", msg.To)
	}

	user := s.users[msg.From]
	if user != nil {
		user.LastActiveTime = time.Now()
		if !bytes.Equal(user.UdpAddr.IP, packet.FromUdpAddr.IP) || user.UdpAddr.Port != packet.FromUdpAddr.Port {
			if msg.From != -1 { //session manager可能有多个ip地址，所以这里不予考虑
				logging.Logger.Warn("received signal from user ", msg.From, " with changed udp address:", packet.FromUdpAddr.String(), " origin:", user.UdpAddr.String())
				user.UdpAddr = packet.FromUdpAddr
			}
		}
	} else {
		logging.Logger.Warn("user ", msg.From, " not existed")
	}

	user = s.users[msg.To]

	if user != nil {
		s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), user.UdpAddr)

		if signal.Signal != YCKCallSignalTypeStateSync && signal.Signal != YCKCallSignalTypeStateInfo {
			logging.Logger.Info("route user signal", signal.String(), " From ", msg.From, " To ", msg.To, "<", user.UdpAddr.String(), ">")
		}
	} else {
		logging.Logger.Warn("user ", msg.To, " not existed")
	}
}

//清理过期的session和user
func (s *Service) handleTicker(now time.Time) {
	for skey, session := range s.sessions {
		for pkey, participant := range session.Participants {
			if now.Sub(participant.LastActiveTime) > 120*time.Second {
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

	if len(s.sessions) > 0 || len(s.users) > 0 {
		logging.Logger.Infoln("details:")
		for skey, session := range s.sessions {
			logging.Logger.Info("    session: ", skey)
			for pkey, _ := range session.Participants {
				logging.Logger.Info("       participant:", pkey)
			}
		}

		for ukey, _ := range s.users {
			logging.Logger.Info("    reg user:", ukey)
		}
	}

	//utils.PrintMemUsage()
}
