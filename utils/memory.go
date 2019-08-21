/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package utils

import (
	"fmt"
	"runtime"
)

func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
	fmt.Printf("\tMallocs = %v MiB", bToMb(m.Mallocs))
	fmt.Printf("\tFrees = %v MiB", bToMb(m.Frees))
	fmt.Printf("\tHeapObjects = %v", m.HeapObjects)
	fmt.Printf("\tPauseTotalNs = %v", m.PauseTotalNs)
	fmt.Printf("\tNumGC = %v", m.NumGC)
	fmt.Printf("\tNumGoroutine = %v\n", runtime.NumGoroutine())
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
