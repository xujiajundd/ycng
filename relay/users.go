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
	uid            uint64
	addr           *net.UDPAddr
	lastActiveTime *time.Time
}
