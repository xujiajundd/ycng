/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package utils

import (
	"crypto/rand"
	"io"
	"testing"

	"fmt"
	"github.com/xujiajundd/ycng/utils/logging"
)

func TestObfuscateData(t *testing.T) {
	//generateObfDict(8192)
	for i := 0; i < 10; i++ {
		data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		obf := ObfuscateData(data)
		data2 := DataFromObfuscated(obf)

		fmt.Println(obf)
		fmt.Println(data2)
	}
}

func generateObfDict(len int) {
	buf := make([]byte, len)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		logging.Logger.Error("reading from crypto/rand failed: " + err.Error())
	}

	for i := 0; i < len/16; i++ {
		for j := 0; j < 16; j++ {
			p := 16*i + j
			fmt.Printf("0x%02x, ", buf[p])
		}
		fmt.Println()
	}
}
