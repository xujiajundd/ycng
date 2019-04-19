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
)

type Service struct {
	config     *Config
	sessions   map[uint64]*Session
	udp_server *UdpServer
	tcp_server *TcpServer
	packetReceiveCh chan *ReceivedPacket

	isRunning  bool
	lock       sync.RWMutex
	stop       chan struct{}
	wg      sync.WaitGroup
}

func NewService(config *Config) *Service {
	service := &Service{
		config:    config,
		isRunning: false,
	}
	return service
}

func (s *Service) Start() (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.isRunning {

		s.isRunning = true
	}
	return nil
}

func (s *Service) Stop() (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.isRunning {
		s.isRunning = false
	}
	close(s.stop)
	return nil
}

func (s *Service) loop() {
	s.wg.Add(1)
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
