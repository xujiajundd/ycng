/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import "github.com/urfave/cli"

type Config struct {
	Dir string `toml:"dir"`
	UdpAddr string `toml:"udp_addr"`
}

func GetConfig(ctx *cli.Context) *Config {
	config := GetDefaultConfig()

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
