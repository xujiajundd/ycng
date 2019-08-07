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
	Uid           uint64
	State         uint16
	Event         uint16
	LastStateTime time.Time
	//option,info,device info之类信息需要补充
}

func NewParticipant(uid uint64) *Participant {
	p := &Participant{
		Uid:           uid,
		State:         YCKParticipantStateIdle,
		LastStateTime: time.Now(),
	}
	return p
}

func (p *Participant) SetState(state uint16) {
	p.State = state
}

func (p *Participant) InState(state uint16) bool {
	return p.State == state
}

func (p *Participant) SetEvent(event uint16) {
	p.Event = event
}

type Session struct {
	Sid            uint64
	Mode           int
	Participants   map[uint64]*Participant
	Relays         []string
	LastActiveTime time.Time
}

func NewSession(sid uint64) *Session {
	s := &Session{
		Sid:            sid,
		Mode:           YCKCallModeUndecided,
		Participants:   make(map[uint64]*Participant),
		LastActiveTime: time.Now(),
	}
	return s
}
