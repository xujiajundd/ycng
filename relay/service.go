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
	"encoding/binary"
	"encoding/json"
	"reflect"
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

	acc_msg map[uint8]int
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
		ticker:          time.NewTicker(30 * time.Second),
		acc_msg:         make(map[uint8]int),
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
		logging.Logger.Warn("error:", err, " for packet received from <", packet.FromUdpAddr.String(), ">")
		return
	}

	s.acc_msg[msg.MsgType]++

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

	case UdpMessageTypeThumbVideoStream:
		s.handleMessageVideoStream(msg, packet)

	case UdpMessageTypeThumbVideoStreamIFrame:
		s.handleMessageVideoStreamIFrame(msg, packet)

	case UdpMessageTypeThumbVideoNack:
		s.handleMessageVideoNack(msg, packet)

	case UdpMessageTypeThumbVideoAskForIFrame:
		s.handleMessageVideoASkForIFrame(msg, packet)

	case UdpMessageTypeVideoOnlyIFrame:

	case UdpMessageTypeVideoOnlyAudio:
		s.handleMessageVideoOnlyAudio(msg)

	case UdpMessageTypeUserReg:
		s.handleMessageUserReg(msg, packet)

	case UdpMessageTypeUserSignal:
		s.handleMessageUserSignal(msg, packet)

	case UdpMessageTypeMediaControl:
		s.handleMessageMediaControl(msg, packet)

	default:
		logging.Logger.Warn("unrecognized message type ", msg.MsgType, " from ", msg.From)
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
		participant.ThumbVideoQueueOut = NewQueueOut()
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
	//客户端会重复几次发这条消息，只有必要log一次
	logging.Logger.Info("received turn unreg From ", msg.From, " for session ", msg.To)
	delete(session.Participants, participant.Id)

	////如果剩下的参与方只有两个，也尝试发TurnInfo？
	//if len(session.Participants) == 2 {
	//	//todo
	//}
}

func (s *Service) handleMessageAudioStream(msg *Message, packet *ReceivedPacket) {
	//logging.Logger.Info("received audio From ", msg.From, " To ", msg.To)
	if len(msg.Payload) < 12 {
		logging.Logger.Error("error audio packet from:", msg.From, " to:", msg.To, " payload:", msg.Payload)
		return
	}

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
				participant.PendingTime = time.Now()
			}
			for _, p := range session.Participants {
				if p.Id != msg.From || (p.Id == 0 && msg.From == 0) { //后一个条件是为了本地回环测试，非登录用户的id为0
					//如果p要求了participant发的音频需要有repeat, 则看这个包是否属于重发范围
					//重发范围界定：1）src包，2）src包的esi小于repeat factor.
					repeatFactor := p.AudioRepeatFactor[participant.Id]
					needRepeat := false
					seqid := int16(0)
					seqid = seqid + 1 //这是个废语句，为log中没引用的时候了不报错
					esi := 0
					if repeatFactor >= 0 && repeatFactor <= 5 {
						esi = int(binary.BigEndian.Uint16(msg.Payload[9:11]))
						seqid = int16(binary.BigEndian.Uint16(msg.Payload[0:2]))
						if repeatFactor >= 3 {
							if esi < repeatFactor {
								needRepeat = true
							}
						} else if repeatFactor == 2 {
							if esi == 0 || esi == 2 {
								needRepeat = true
							}
						} else if repeatFactor == 1 {
							if esi == 1 {
								needRepeat = true
							}
						} else if repeatFactor == 0 {
							//p针对participant的audio没有重发要求
						}
					} else {
						logging.Logger.Warn("incorrect audio repeatFactor:", repeatFactor, " for ", participant.Id, " of receiver ", p.Id)
						delete(p.AudioRepeatFactor, participant.Id) //清除为0防止无休止打日志
					}
					if needRepeat {
						msg.Tseq = p.Tseq
						p.Tseq++
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), p.UdpAddr)
						//logging.Logger.Info("repeat audio packet ", seqid, esi, " from ", participant.Id, " to ", p.Id)
					} else {
						if p.PendingMsg == nil {
							p.PendingMsg = msg
						} else {
							p.PendingMsg.Tseq = p.Tseq
							msg.Tseq = p.Tseq
							p.Tseq++
							extraAdded := false
							if p.PendingExtra != nil && msg.Extra == nil {
								now := time.Now()
								delay := now.Sub(p.PendingTime) / time.Millisecond
								if delay < 750 { //这个地方原来协议只留了一个字节，不够用，所以用这种方法放大一点点。
									if delay > 200 {
										delay = 200 + (delay-200)/10
									}
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
			}
		} else {
			logging.Logger.Info("participant ", msg.From, " not existed in session ", msg.To, " send audio packet")
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session ", msg.To, " not existed for audio packet from ", msg.From)
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
				participant.PendingTime = time.Now()
			}
			if msg.MsgType == UdpMessageTypeVideoStream {
				participant.VideoQueueOut.AddItem(false, msg.Payload, msg.From)
			} else if msg.MsgType == UdpMessageTypeThumbVideoStream {
				participant.ThumbVideoQueueOut.AddItem(false, msg.Payload, msg.From)
			} else {
				logging.Logger.Warn("incorrect message type for video stream")
			}

			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.OnlyAcceptAudio {
					continue
				}
				//如果没有列表请求数据，那么为兼容老版本，9方以下还发视频。如果已经有这个列表，则按列表规则发
				if msg.MsgType == UdpMessageTypeVideoStream {
					if p.VideoList == nil {
						if len(session.Participants) > 12 { //TODO:这个在客户端都升级后，需要改
							continue
						}
					} else {
						if p.VideoList[participant.Id] < 1 {
							continue
						}
					}
				} else if msg.MsgType == UdpMessageTypeThumbVideoStream {
					if p.ThumbVideoList == nil {
						continue
					} else {
						if p.ThumbVideoList[participant.Id] < 1 {
							continue
						}
					}
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
							delay := now.Sub(p.PendingTime) / time.Millisecond
							if delay < 750 { //这个地方原来协议只留了一个字节，不够用，所以用这种方法放大一点点。
								if delay > 200 {
									delay = 200 + (delay-200)/10
								}
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
			logging.Logger.Info("participant ", msg.From, " not existed in session ", msg.To, " send video packet")
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session ", msg.To, " not existed for video packet from ", msg.From)
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
				participant.PendingTime = time.Now()
			}
			if msg.MsgType == UdpMessageTypeVideoStreamIFrame {
				participant.VideoQueueOut.AddItem(true, msg.Payload, msg.From)
			} else if msg.MsgType == UdpMessageTypeThumbVideoStreamIFrame {
				participant.ThumbVideoQueueOut.AddItem(true, msg.Payload, msg.From)
			} else {
				logging.Logger.Warn("incorrect message type for video stream iframe")
			}

			for _, p := range session.Participants {
				if msg.Dest != 0 && p.Id != msg.Dest {
					continue
				}
				if p.OnlyAcceptAudio {
					continue
				}
				if msg.MsgType == UdpMessageTypeVideoStreamIFrame {
					if p.VideoList == nil {
						if len(session.Participants) > 12 { //TODO:这个在客户端都升级后，需要改
							continue
						}
					} else {
						if p.VideoList[participant.Id] < 1 {
							continue
						}
					}
				} else if msg.MsgType == UdpMessageTypeThumbVideoStreamIFrame {
					if p.ThumbVideoList == nil {
						continue
					} else {
						if p.ThumbVideoList[participant.Id] < 1 {
							continue
						}
					}
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
							delay := now.Sub(p.PendingTime) / time.Millisecond
							if delay < 750 { //这个地方原来协议只留了一个字节，不够用，所以用这种方法放大一点点。
								if delay > 200 {
									delay = 200 + (delay-200)/10
								}
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
			logging.Logger.Info("participant ", msg.From, " not existed in session ", msg.To, " send video packet")
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session ", msg.To, " not existed for video packet from ", msg.From)
		s.askForReTurnReg(msg, packet)
	}
}

func (s *Service) handleMessageVideoASkForIFrame(msg *Message, packet *ReceivedPacket) {
	isThumb := ""
	if msg.MsgType == UdpMessageTypeThumbVideoAskForIFrame {
		isThumb = " for thumb"
	}
	logging.Logger.Info("received ask for iframe From ", msg.From, " To ", msg.To, " Dest ", msg.Dest, isThumb)

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
					////如果a向b请求i帧了，那么a的可接收视频列表里也要立即把b列进去，之后客户端会来再刷新的。//这个导致混乱，取消之！
					//if msg.MsgType == UdpMessageTypeVideoAskForIFrame {
					//	if participant.VideoList != nil {
					//		participant.VideoList[p.Id] = 2
					//	}
					//} else if msg.MsgType == UdpMessageTypeThumbVideoAskForIFrame{
					//	if participant.ThumbVideoList != nil {
					//		participant.ThumbVideoList[p.Id] = 2
					//	}
					//} else {
					//	logging.Logger.Warn("incorrect message type for ask for iframe")
					//}
				}
			}
		} else {
			logging.Logger.Info("participant ", msg.From, " not existed in session ", msg.To, " ask iframe")
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session ", msg.To, " not existed for ask iframe packet from ", msg.From)
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
			var queue *QueueOut
			if msg.MsgType == UdpMessageTypeVideoNack {
				queue = dest.VideoQueueOut
			} else if msg.MsgType == UdpMessageTypeThumbVideoNack {
				queue = dest.ThumbVideoQueueOut
			} else {
				logging.Logger.Warn("incorrect message type for video nack")
			}
			seqid, n_tries, isIFrame, packets := queue.ProcessNack(nack, msg.From)
			//logging.Logger.Info("process nack from ", msg.From, " to sid ", msg.To, " dest ", msg.Dest, " seq ", seqid, " n_tries ", n_tries, " packets ", len(packets))

			//报告给metrix汇总打日志
			participant.Metrics.ProcessNack(msg, seqid, n_tries, len(packets), msg.MsgType == UdpMessageTypeThumbVideoNack)

			//从Dest的QueueOut中查找是否可以响应nack
			if packets != nil && len(packets) > 0 {
				for i := 0; i < len(packets); i++ {
					packet := packets[i]
					var nmsgType int
					if msg.MsgType == UdpMessageTypeVideoNack {
						nmsgType = UdpMessageTypeVideoStream
						if isIFrame {
							nmsgType = UdpMessageTypeVideoStreamIFrame
						}
					} else {
						nmsgType = UdpMessageTypeThumbVideoStream
						if isIFrame {
							nmsgType = UdpMessageTypeThumbVideoStreamIFrame
						}
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
			logging.Logger.Info("participant ", msg.From, " not existed in session ", msg.To, " send nack")
			s.askForReTurnReg(msg, packet)
		}
	} else {
		logging.Logger.Info("session ", msg.To, " not existed for nack packet from ", msg.From)
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
				logging.Logger.Warn("participant ", msg.From, " incorrect audio only request")
			}
		} else {
			logging.Logger.Info("participant ", msg.From, " not existed in session ", msg.To)
		}
	} else {
		logging.Logger.Info("session ", msg.To, " not existed for audio only packet from ", msg.From)
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

	if !msg.HasFlag(UdpMessageFlagGZip) {
		err := signal.Unmarshal(msg.Payload)
		if err != nil {
			logging.Logger.Warn("signal unmarshal error:", err, " payload(", len(msg.Payload), "):", string(msg.Payload), " from ", msg.From)
		}

		//State sync和state info两个信令太多，不打在日志之中了。
		if signal.Signal != YCKCallSignalTypeStateSync && signal.Signal != YCKCallSignalTypeStateInfo {
			logging.Logger.Info("received user signal From ", msg.From, "<", packet.FromUdpAddr.String(), ">", " To ", msg.To)
		}
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
		logging.Logger.Warn("user ", msg.From, " not existed in signal msg.from， register the user ", "<", packet.FromUdpAddr.String(), ">",)
		user = NewUser(msg.From)
		s.users[msg.From] = user
		user.UdpAddr = packet.FromUdpAddr
		user.LastActiveTime = time.Now()
	}

	user = s.users[msg.To]

	if user != nil {
		s.udp_server.SendPacket(msg.ObfuscatedDataOfMessage(), user.UdpAddr)
		if !msg.HasFlag(UdpMessageFlagGZip) {
			if signal.Signal != YCKCallSignalTypeStateSync && signal.Signal != YCKCallSignalTypeStateInfo {
				logging.Logger.Info("route user signal", signal.String(), " From ", msg.From, " To ", msg.To, "<", user.UdpAddr.String(), ">")
			}
		}
	} else {
		logging.Logger.Warn("user ", msg.To, " not existed in signal msg.to", signal.String())
	}
}

func (s *Service) handleMessageMediaControl(msg *Message, packet *ReceivedPacket) {
	session := s.sessions[msg.To]

	if session != nil {
		participant := session.Participants[msg.From]
		if participant != nil {
			payload := msg.Payload
			len := len(payload)
			if len < 3 {
				logging.Logger.Info("participant ", msg.From, " incorrect media control message ", payload)
				return
			}
			p := 0
			for p <= len-3 {
				size := binary.BigEndian.Uint16(payload[p : p+2])
				p += 2
				key := payload[p]
				p += 1
				if p+int(size) > len {
					logging.Logger.Info("participant ", msg.From, " incorrect media control message ", payload)
					return
				}

				value := payload[p : p+int(size)]
				if key == 1 { //视频请求列表uid
					if size%8 != 0 {
						logging.Logger.Info("participant ", msg.From, "error value size for key ", key, " for media control message ", payload)
					}
					num := int(size) / 8
					uids := make(map[int64]int)
					for i := 0; i < num; i++ {
						uid := int64(binary.BigEndian.Uint64(value[8*i : 8*i+8]))
						uids[uid] = 1
					}

					if !reflect.DeepEqual(uids, participant.VideoList) {
						logging.Logger.Info(msg.From, " media control video: ", uids)
					}
					participant.VideoList = uids

				} else if key == 2 { //缩略视频请求列表uid
					if size%8 != 0 {
						logging.Logger.Info("participant ", msg.From, "error value size for key ", key, " for media control message ", payload)
					}
					num := int(size) / 8
					uids := make(map[int64]int)
					for i := 0; i < num; i++ {
						uid := int64(binary.BigEndian.Uint64(value[8*i : 8*i+8]))
						uids[uid] = 1
					}

					if !reflect.DeepEqual(uids, participant.ThumbVideoList) {
						logging.Logger.Info(msg.From, " media control thumb video: ", uids)
					}
					participant.ThumbVideoList = uids

				} else if key == 3 { //音频补偿系数，0-8，
					if size%9 != 0 {
						logging.Logger.Info("participant ", msg.From, "error value size for key ", key, " for media control message ", payload)
					}
					num := int(size) / 9
					uids := make(map[int64]int)
					for i := 0; i < num; i++ {
						uid := int64(binary.BigEndian.Uint64(value[9*i : 9*i+8]))
						factor := int(value[9*i+8])
						uids[uid] = factor
					}

					if !reflect.DeepEqual(uids, participant.AudioRepeatFactor) {
						logging.Logger.Info(msg.From, " media control audio repeat: ", uids)
					}
					participant.AudioRepeatFactor = uids

				} else {
					logging.Logger.Info("participant ", msg.From, "unknown key ", key, " for media control message ", payload)
				}

				p += int(size)
			}
		} else {
			logging.Logger.Info("participant ", msg.From, " not existed in session ", msg.To, " for media control message")
		}
	} else {
		logging.Logger.Info("session ", msg.To, " not existed for media control packet from ", msg.From)
	}
}

//清理过期的session和user
var tickCount = 0

func (s *Service) handleTicker(now time.Time) {
	numSessions := 0
	numParticipants := 0
	numRegUsers := 0
	for skey, session := range s.sessions {
		for pkey, participant := range session.Participants {
			if now.Sub(participant.LastActiveTime) > 45*time.Second { //因为给非活跃relay客户端也会定期发小包，所以这儿超时可以缩短
				delete(session.Participants, pkey)
				logging.Logger.Info("delete participant ", pkey, " From session ", skey, " for inactive 45s")
			} else {
				numParticipants++
			}
		}
		if len(session.Participants) == 0 {
			delete(s.sessions, skey)
			logging.Logger.Info("delete session ", skey, " for all participants quit")
		} else {
			numSessions++
		}
	}

	for ukey, user := range s.users {
		if now.Sub(user.LastActiveTime) > 600*time.Second {
			delete(s.users, ukey)
			logging.Logger.Info("delete user ", ukey, " for inactive 10 minutes")
		} else {
			numRegUsers++
		}
	}

	tickCount++
	if tickCount%2 == 0 {
		logging.Logger.Info("<<< current active sessions:", numSessions, " participants:", numParticipants, " reg users:", numRegUsers, " >>>")
	}
	if tickCount%20 == 0 { //每十分钟打印一次
		if len(s.sessions) > 0 || len(s.users) > 0 {
			logging.Logger.Infoln("details:")
			for skey, session := range s.sessions {
				logging.Logger.Info("    session: ", skey)
				for pkey, p := range session.Participants {
					logging.Logger.Info("       participant:", pkey, "<", p.UdpAddr.String() ,">")
				}
			}

			for ukey, u := range s.users {
				logging.Logger.Info("    reg user:", ukey, "<", u.UdpAddr.String(), ">")
			}
		}

		logging.Logger.Info("    messages sum:")
		logging.Logger.Info("        sum noop:          ", s.acc_msg[UdpMessageTypeNoop])
		logging.Logger.Info("        sum turn reg:      ", s.acc_msg[UdpMessageTypeTurnReg])
		logging.Logger.Info("        sum turn unreg:    ", s.acc_msg[UdpMessageTypeTurnUnReg])
		logging.Logger.Info("        sum audio:         ", s.acc_msg[UdpMessageTypeAudioStream])
		logging.Logger.Info("        sum video:         ", s.acc_msg[UdpMessageTypeVideoStream])
		logging.Logger.Info("        sum video iframe:  ", s.acc_msg[UdpMessageTypeVideoStreamIFrame])
		logging.Logger.Info("        sum video nack:    ", s.acc_msg[UdpMessageTypeVideoNack])
		logging.Logger.Info("        sum video ask i:   ", s.acc_msg[UdpMessageTypeVideoAskForIFrame])
		logging.Logger.Info("        sum thumb video:   ", s.acc_msg[UdpMessageTypeThumbVideoStream])
		logging.Logger.Info("        sum thumb video i: ", s.acc_msg[UdpMessageTypeThumbVideoStreamIFrame])
		logging.Logger.Info("        sum thumb video n: ", s.acc_msg[UdpMessageTypeThumbVideoNack])
		logging.Logger.Info("        sum thumb video a: ", s.acc_msg[UdpMessageTypeThumbVideoAskForIFrame])
		logging.Logger.Info("        sum user reg:      ", s.acc_msg[UdpMessageTypeUserReg])
		logging.Logger.Info("        sum user signal:   ", s.acc_msg[UdpMessageTypeUserSignal])
		logging.Logger.Info("        sum media control: ", s.acc_msg[UdpMessageTypeMediaControl])

		for k, _ := range s.acc_msg {
			s.acc_msg[k] = 0
		}
	}
	//utils.PrintMemUsage()
}
