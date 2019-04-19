/*
 * // Copyright (C) 2017 yeecall authors
 * //
 * // This file is part of the yeecall library.
 *
 */

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/urfave/cli"
	"github.com/xujiajundd/ycng/utils/logging"
	"github.com/xujiajundd/ycng/relay"
)

var app = cli.NewApp()

func init() {
	app.Name = filepath.Base(os.Args[0])
	app.Author = ""
	app.Email = ""
	app.Version = ""
	app.Usage = "Relay"
	app.HideVersion = true
	app.Copyright = "Copyright 2017-2018 The yeecall Authors"

	app.Action = Relay
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	if err := app.Run(os.Args); err != nil {
		logging.Logger.Fatal(err)
	}
}

func Relay(ctx *cli.Context) error {
	config := relay.GetConfig(ctx)
    service := relay.NewService(config)
    service.Start()
    service.WaitForShutdown()
    return nil
}
