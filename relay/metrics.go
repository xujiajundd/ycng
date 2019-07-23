/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"time"
	"github.com/xujiajundd/ycng/utils/logging"
)

type UmsgStat struct {
	paired    bool
	tid       uint8
	tseq      int16
	bytes     uint16
	timestamp int64
}

type Metrics struct {
	stat [2000]UmsgStat
	pos    int
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		stat: [2000]UmsgStat{},
		pos:    0,
	}

	return metrics
}

func(m *Metrics) AddEntry(tid uint8, tseq int16, bytes uint16) {
	m.stat[m.pos].paired = false
	m.stat[m.pos].tid = tid
	m.stat[m.pos].tseq = tseq
	m.stat[m.pos].bytes = bytes
	m.stat[m.pos].timestamp = time.Now().UnixNano()

	if m.pos < 1999 {
		m.pos++
	} else {
		m.pos = 0
	}
}

func(m *Metrics) Process() {
    minSeq := int16(0)
    maxSeq := int16(0)
	rept := 0
	accPairs := 0
	accBytes := uint32(0)
	accTimes := int64(0)

	for p:=0; p<m.pos; p++ {
		u1 := m.stat[p];
		if minSeq == 0 && maxSeq == 0 {
			minSeq = u1.tseq
			maxSeq = u1.tseq
		} else {
			if int16(u1.tseq - maxSeq) > 0 {
				maxSeq = u1.tseq
			}
			if int16(u1.tseq - minSeq) < 0 {
				minSeq = u1.tseq
			}

		}

		for q:=p+1; q<p+10 && q < m.pos; q++ {
			if u1.tseq == m.stat[q].tseq {
				m.stat[q].paired = true
				if !u1.paired {
					accPairs++
					accBytes += uint32(m.stat[q].bytes)
					accTimes += m.stat[q].timestamp - u1.timestamp
					break
				} else {
					rept++
				}
			}
		}
	}


	//计算结果
	pShould := 2 * (maxSeq - minSeq) + 2
	if pShould < 0 {
		pShould = 0
	}
	pRecv := m.pos - rept

	bandwidth := 0
	if accPairs > 2 {
		bandwidth = int(8 * int64(accBytes) * int64(time.Second) / int64(accTimes) / 1024)
	}

	logging.Logger.Info("应收包:", pShould, " 实收包:", pRecv, " 重复:", rept, " 带宽:", bandwidth)

	m.pos = 0
}
