/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"github.com/urfave/cli"
	"fmt"
)

type Config struct {
	Dir string `toml:"dir"`
	UdpAddr string `toml:"udp_addr"`
}

func GetConfig(ctx *cli.Context) *Config {
	config := GetDefaultConfig()
    if ctx.GlobalIsSet("port") {
    	config.UdpAddr = fmt.Sprintf(":%d", ctx.GlobalInt("port"))
	}
	return config
}

func GetDefaultConfig() *Config {
	var config *Config

	config = &Config{
		Dir: "",
		UdpAddr:":19001",
	}
	return config
}
