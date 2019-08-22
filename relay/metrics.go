/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"github.com/xujiajundd/ycng/utils/logging"
	"time"
	"encoding/binary"
)

const  StatBufferSize = 100

type UmsgStat struct {
	paired    bool
	tid       uint8
	tseq      int16
	bytes     uint16
	timestamp int64
}

type Metrics struct {
	stat [StatBufferSize]UmsgStat
	pos  int
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		stat: [StatBufferSize]UmsgStat{},
		pos:  0,
	}

	return metrics
}

func (m *Metrics) Process(msg *Message) (ok bool, data []byte) {
	m.stat[m.pos].paired = false
	m.stat[m.pos].tid = msg.Tid
	m.stat[m.pos].tseq = msg.Tseq
	m.stat[m.pos].bytes = msg.NetTrafficSize()
	m.stat[m.pos].timestamp = time.Now().UnixNano()

	m.pos++
	if m.pos >= StatBufferSize {
		minSeq := int16(0)
		maxSeq := int16(0)
		rept := 0
		accPairs := 0
		accBytes := uint32(0)
		accTimes := int64(0)

		for p := 0; p < m.pos; p++ {
			u1 := m.stat[p]
			if minSeq == 0 && maxSeq == 0 {
				minSeq = u1.tseq
				maxSeq = u1.tseq
			} else {
				if int16(u1.tseq-maxSeq) > 0 {
					maxSeq = u1.tseq
				}
				if int16(u1.tseq-minSeq) < 0 {
					minSeq = u1.tseq
				}

			}

			for q := p + 1; q < p+10 && q < m.pos; q++ {
				if u1.tseq == m.stat[q].tseq {
					m.stat[q].paired = true
					if !u1.paired {
						accPairs++
						accBytes += uint32(m.stat[q].bytes)  //这里的假设是relay自己的下行带宽足够，而计算客户端的上行带宽
						accTimes += m.stat[q].timestamp - u1.timestamp
						break
					} else {
						rept++
					}
				}
			}
		}

		//计算结果
		pRecv := m.pos - rept
		m.pos = 0
		pShould := 2*(maxSeq-minSeq) + 2
		if pShould < 0 || (minSeq == 0 && maxSeq == 0) {
			pShould = 0
		}

		bandwidth := 0
		if accPairs > 2 {
			bandwidth = int(8 * int64(accBytes) * int64(time.Second) / int64(accTimes) / 1024)
		}

		logging.Logger.Info(msg.From, " 应收包:", pShould, " 实收包:", pRecv, " 重复:", rept, " 带宽:", bandwidth)

		if pShould > 0 && bandwidth > 0 {
           data = make([]byte, 17)
           data[0] = UdpMessageExtraTypeMetrix
           data[1] = YCKMetrixDataTypeBandwidthUp
           binary.BigEndian.PutUint16(data[2:4], 5)
           data[4] = msg.Tid
           binary.BigEndian.PutUint32(data[5:9], uint32(bandwidth))
           data[9] = YCKMetrixDataTypeLossrateUp
           binary.BigEndian.PutUint16(data[10:12], 5)
           data[12] = msg.Tid
           binary.BigEndian.PutUint16(data[13:15], uint16(pShould))
           binary.BigEndian.PutUint16(data[15:17], uint16(pRecv))
           return true, data
		}
	}

	return false, nil
}

