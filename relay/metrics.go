/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package relay

import (
	"encoding/binary"
	"github.com/xujiajundd/ycng/utils/logging"
	"time"
)

const StatBufferSize = 100

type UmsgStat struct {
	paired    bool
	tid       uint8
	tseq      int16
	bytes     uint16
	timestamp int64
}

type Metrics struct {
	stat          [StatBufferSize]UmsgStat
	pos           int
	lastTimestamp int64
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		stat:          [StatBufferSize]UmsgStat{},
		pos:           0,
		lastTimestamp: time.Now().UnixNano(),
	}

	return metrics
}

func (m *Metrics) Process(msg *Message, timestamp int64) (ok bool, data []byte) {
	m.stat[m.pos].paired = false
	m.stat[m.pos].tid = msg.Tid
	m.stat[m.pos].tseq = msg.Tseq
	m.stat[m.pos].bytes = msg.NetTrafficSize()
	currentTimestamp := timestamp
	m.stat[m.pos].timestamp = currentTimestamp

	m.pos++
	if m.pos >= StatBufferSize || (currentTimestamp-m.lastTimestamp) > int64(time.Second) {
		m.lastTimestamp = currentTimestamp
		minSeq := int16(0)
		maxSeq := int16(0)
		packetDup := 0
		accPairs := 0
		accBytes := uint32(0)
		accTimes := int64(0)
		totalBytes := 0
		totalTime := 0

		for p := 0; p < m.pos; p++ {
			u1 := m.stat[p]
			totalBytes += int(u1.bytes)

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
				if u1.tid != m.stat[q].tid {
					logging.Logger.Error("error:有不一致的tid")
				}
				if u1.tseq == m.stat[q].tseq {
					if !u1.paired {
						u1.paired = true
						m.stat[q].paired = true
						accPairs++
						accBytes += uint32(m.stat[q].bytes) //这里的假设是relay自己的下行带宽足够，而计算客户端的上行带宽
						accTimes += m.stat[q].timestamp - u1.timestamp
						break
					} else {
						if !m.stat[q].paired {
							m.stat[q].paired = true
							packetDup++
						}
					}
				}
			}
		}

		//计算结果
		packetRecv := m.pos - packetDup
		totalTime = int((m.stat[m.pos-1].timestamp - m.stat[0].timestamp) / 1000000) //毫秒时间

		packetShould := 2*(maxSeq-minSeq)
		if packetShould < 0 || (minSeq == 0 && maxSeq == 0) {
			packetShould = 0
		}

		bandwidth := 0
		if accPairs > 2 && accTimes != 0 {
			bandwidth = int(8 * int64(accBytes) * int64(time.Second) / int64(accTimes) / 1024)
		}

		logging.Logger.Info(msg.From, " 应收包:", packetShould, " 实收包:", packetRecv, " 重复:", packetDup, " 带宽:", bandwidth, " pairs:", accPairs)

		if packetShould > 0 && bandwidth > 0 {
			data = make([]byte, 19)
			data[0] = UdpMessageExtraTypeMetrix
			data[1] = YCKMetrixDataTypeBandwidthUp
			data[2] = msg.Tid
			binary.BigEndian.PutUint32(data[3:7], uint32(bandwidth))
			data[7] = YCKMetrixDataTypeLossrateUp
			data[8] = msg.Tid
			binary.BigEndian.PutUint16(data[9:11], uint16(packetShould))
			binary.BigEndian.PutUint16(data[11:13], uint16(packetRecv))
			//data[13] = YCKMetrixDataTypeRealTrafficUp
			//data[14] = msg.Tid
			binary.BigEndian.PutUint32(data[13:17], uint32(totalBytes))
			binary.BigEndian.PutUint16(data[17:19], uint16(totalTime))

			m.pos = 0
			return true, data
		}
	}

	return false, nil
}
