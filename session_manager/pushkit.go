/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package session_manager

import (
	"fmt"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/certificate"
	"github.com/xujiajundd/ycng/utils/logging"
	"github.com/xujiajundd/ycng/utils/res"
)

type Pushkit struct {
	Client *apns2.Client
}

func NewPushkit() *Pushkit {
	pk := &Pushkit{}

	certbyte := res.MustAsset("cert/yckit_demo.p12")

	cert, err := certificate.FromP12Bytes(certbyte, "")
	if err != nil {
		logging.Logger.Fatal("Cert Error:", err)
	}

	pk.Client = apns2.NewClient(cert).Development()

	return pk
}

func (pk *Pushkit) Push(token string, payload []byte) {
	notification := &apns2.Notification{}
	notification.Topic = "com.yeecall.YCKitDemo.voip"

	notification.DeviceToken = token
	notification.Payload = payload //[]byte(`{"aps":{"alert":"Hello!"}}`)

	res, err := pk.Client.Push(notification)

	if err != nil {
		logging.Logger.Fatal("Error:", err)
	}

	fmt.Printf("pushkit ret: %v %v %v\n", res.StatusCode, res.ApnsID, res.Reason)
}
