/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package session_manager

import "time"

const (
	YCKCallModeOneToOne = 1
	YCKCallModeMultiple = 2
)

type Participant struct {
	Uid uint64

}

type Session struct {
	Sid            uint64
	Mode           int
	Participants   map[uint64]*Participant
	LastActiveTime time.Time
}

func NewSession(sid uint64) *Session {
	s := &Session{
		Sid: sid,
	}
	return s
}
