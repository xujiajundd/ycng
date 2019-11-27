/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package session_manager

import "time"

const (
	YCKCallModeUndecided = 0
	YCKCallModeOneToOne  = 1
	YCKCallModeMultiple  = 2

	YCKParticipantStateIdle     = 0
	YCKParticipantStateCalling  = 1
	YCKParticipantStateCalled   = 2
	YCKParticipantStateIncall   = 4

	YCKParticipantEventInvite     = 1
	YCKParticipantEventRecvInvite = 2
	YCKParticipantEventCancel     = 3
	YCKParticipantEventRecvCancel = 4
	YCKParticipantEventAccept     = 5
	YCKParticipantEventRecvAccept = 6
	YCKParticipantEventReject     = 7
	YCKParticipantEventRecvReject = 8
	YCKParticipantEventBusy       = 9
	YCKParticipantEventRecvBusy   = 10
	YCKParticipantEventEnd        = 11
	YCKParticipantEventRecvEnd    = 12
	YCKParticipantEventTimout     = 13
)

type Participant struct {
	Uid           int64
	State         uint16
	Event         uint16
	LastStateTime time.Time
	Timeout      *time.Timer
	HasChange     bool
	//option,info,device info之类信息需要补充
}

func NewParticipant(uid int64) *Participant {
	p := &Participant{
		Uid:           uid,
		State:         YCKParticipantStateIdle,
		LastStateTime: time.Now(),
		HasChange:     false,
	}
	return p
}

func (p *Participant) SetState(state uint16) {
	p.State = state
	p.HasChange = true
	if p.Timeout != nil {
		p.Timeout.Stop()
	}
}

func (p *Participant) InState(state uint16) bool {
	return p.State == state
}

func (p *Participant) SetEvent(event uint16) {
	p.Event = event
}

func (p *Participant) setCallingTimeout(duration time.Duration, f func()) {
	p.Timeout = time.AfterFunc(duration, f)
}

type Session struct {
	Sid            int64
	Mode           int
	Participants   map[int64]*Participant
	Relays         []string
	LastActiveTime time.Time
	Nickname       string   //这个多方通话的昵称，在invite其他member的信令消息中应该需要用到
}

func NewSession(sid int64) *Session {
	s := &Session{
		Sid:            sid,
		Mode:           YCKCallModeUndecided,
		Participants:   make(map[int64]*Participant),
		LastActiveTime: time.Now(),
	}
	return s
}
