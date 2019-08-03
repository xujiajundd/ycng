/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"bufio"
	"fmt"
	"github.com/urfave/cli"
	"github.com/xujiajundd/ycng/relay"
	"github.com/xujiajundd/ycng/utils/logging"
	"net"
	"strconv"
	"strings"
)

var client = cli.NewApp()

func init() {
	client.Name = filepath.Base(os.Args[0])
	client.Author = ""
	client.Email = ""
	client.Version = ""
	client.Usage = "Client"
	client.HideVersion = true
	client.Copyright = "Copyright 2017-2018 The yeecall Authors"
	client.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "saddr",
			Value: "127.0.0.1:19001",
			Usage: "server address",
		},
	}
	client.Action = Client
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	if err := client.Run(os.Args); err != nil {
		logging.Logger.Fatal(err)
	}
}

func Client(ctx *cli.Context) error {
	saddr := ctx.GlobalString("saddr")

	udpAddr, err := net.ResolveUDPAddr("udp4", saddr)
	if err != nil {
		logging.Logger.Error("incorrect addr ", err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		logging.Logger.Error("error dialudp ", err)
	}

	go readServer(conn)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("$: ")
		cmdString, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}

		cmdString = strings.TrimSpace(cmdString)

		if cmdString == "exit" {
			break
		}

		err = runCommand(cmdString, conn)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	return nil
}

func runCommand(commandStr string, conn *net.UDPConn) error {
	cs := strings.Split(commandStr, " ")
	if cs[0] == "reg" && len(cs) == 3 { // reg id session
		from, _ := strconv.ParseUint(cs[1], 10, 64)
		to, _ := strconv.ParseUint(cs[2], 10, 64)

		msg := relay.NewMessage(relay.UdpMessageTypeTurnReg,
			from, to, 0, nil,nil)
		data := msg.ObfuscatedDataOfMessage()

		_, err := conn.Write(data)
		if err != nil {
			logging.Logger.Error("udp write error", err)
		}

	} else if cs[0] == "send" && len(cs) == 4 { // send id session txt
		from, _ := strconv.ParseUint(cs[1], 10, 64)
		to, _ := strconv.ParseUint(cs[2], 10, 64)
		payload := []byte(cs[3])

		msg := relay.NewMessage(relay.UdpMessageTypeAudioStream,
			from, to, 0, payload, nil)
		data := msg.ObfuscatedDataOfMessage()

		_, err := conn.Write(data)
		if err != nil {
			logging.Logger.Error("udp write error", err)
		}
	} else {

	}

	return nil
}

func readServer(conn *net.UDPConn) {
	var buf [2048]byte
	for {
		n, err := conn.Read(buf[0:])
		if err != nil {
			logging.Logger.Error("conn read error ", err)
		}

		data := buf[0:n]

		msg, err := relay.NewMessageFromObfuscatedData(data)
         if err != nil {
         	logging.Logger.Error("message from obf error ", err)
		 }

		 fmt.Println("recv:", string(msg.Payload))
	}
}
