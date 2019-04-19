/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package logging

import (
	"github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"time"
)

var Logger *logrus.Logger

func init() {
	Logger = logrus.New()
	Logger.Out = os.Stdout
	Logger.Formatter = &logrus.TextFormatter{FullTimestamp: true}
	Logger.Level = logrus.InfoLevel
}

func SetFileRotationHooker(path string, count uint) {
	frHook := newFileRotateHooker(path, count)
	Logger.Hooks.Add(frHook)
}

func newFileRotateHooker(path string, count uint) logrus.Hook {
	if len(path) == 0 {
		panic("Failed to parse logger folder:" + path + ".")
	}
	if !filepath.IsAbs(path) {
		path, _ = filepath.Abs(path)
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		panic("Failed to create logger folder:" + path + ". err:" + err.Error())
	}
	filePath := path + "/yee-%Y%m%d-%H.log"
	linkPath := path + "/yee.log"
	writer, err := rotatelogs.New(
		filePath,
		rotatelogs.WithLinkName(linkPath),
		rotatelogs.WithRotationTime(time.Duration(24)*time.Hour),
		rotatelogs.WithRotationCount(count),
	)

	if err != nil {
		panic("Failed to create rotate logs. err:" + err.Error())
	}

	hook := lfshook.NewHook(lfshook.WriterMap{
		logrus.DebugLevel: writer,
		logrus.InfoLevel:  writer,
		logrus.WarnLevel:  writer,
		logrus.ErrorLevel: writer,
		logrus.FatalLevel: writer,
	}, nil)
	return hook
}
