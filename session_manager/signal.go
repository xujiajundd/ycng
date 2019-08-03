/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package session_manager

type Signal struct {
    Category  uint16   `json:"c"`
    Signal    uint16   `json:"g"`
    Timestamp uint64   `json:"ts"`
    SessionId uint64   `json:"s"`
    From      uint64   `json:"f"`
    To        uint64   `json:"t"`
    Ttl       uint32   `json:"l"`
}

func NewSignal() *Signal {
	s := &Signal{}
	return s
}