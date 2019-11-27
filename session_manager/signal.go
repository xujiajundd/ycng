/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package session_manager

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/xujiajundd/ycng/utils/logging"
)

const (
	YCKSignalCategoryCall = 1

	YCKCallSignalTypeInvite             = 1
	YCKCallSignalTypeSidRequest         = 2
	YCKCallSignalTypeSidCreated         = 3
	YCKCallSignalTypeRing               = 4
	YCKCallSignalTypeServerRing         = 5
	YCKCallSignalTypeAccept             = 6
	YCKCallSignalTypeReject             = 7
	YCKCallSignalTypeCancel             = 8
	YCKCallSignalTypeEnd                = 9
	YCKCallSignalTypeBusy               = 10
	YCKCallSignalTypeMemberOp           = 20
	YCKCallSignalTypeMemberState        = 21
	YCKCallSignalTypeMemberStateRequest = 22

	YCKCallSignalTypeVoipTokenReg = 100 //严格来讲，这个不是一个call信令，姑且用之。。。
)

type Signal struct {
	Category  uint16                 `json:"c"`
	Signal    uint16                 `json:"g"`
	Timestamp int64                  `json:"ts"`
	SessionId int64                  `json:"s"`
	From      int64                  `json:"f"`
	To        int64                  `json:"t"`
	Ttl       uint32                 `json:"l"`
	Option    map[string]interface{} `json:"o,omitempty"`
	Info      map[string]interface{} `json:"i,omitempty"`
}

func NewSignalTemp() *Signal {
	s := &Signal{}

	return s
}

func NewSignal(signal uint16, from int64, to int64, sid int64) *Signal {
	s := NewSignalTemp()
	s.Category = YCKSignalCategoryCall
	s.Timestamp = int64(time.Now().UnixNano()) / 100000
	s.Signal = signal
	s.From = from
	s.To = to
	s.SessionId = sid
	s.Ttl = 60000
	return s
}

func (s *Signal) Unmarshal(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err := decoder.Decode(s)
	if err != nil {
		return err
	}
	logging.Logger.Info(string(data))
	logging.Logger.Info("receive:", s)

	return nil
}

func (s *Signal) Marshal() ([]byte, error) {
	data, err := json.Marshal(s)
	logging.Logger.Info("send:", string(data))
	return data, err
}
