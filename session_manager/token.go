/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package session_manager

type PushToken struct {
    UserId      uint64
    Token       string
    Platform    string
}

func NewPushToken(uid uint64, token string, platform string) *PushToken {
	pt := &PushToken{
		UserId: uid,
		Token: token,
		Platform: platform,
	}

	return pt
}