/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"net"
	"time"
)

type User struct {
	Uid                uint64
	UdpAddr           *net.UDPAddr
	LastActiveTime     time.Time
}

func NewUser(id uint64) *User {
	user := &User{
		Uid: id,
	}

	return user
}