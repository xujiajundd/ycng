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

const StatBufferSize = 120

type MetrixDataUp struct {
	Tid    uint8
	Bytes  int32
	Times   int16
	Bandwidth  int32
	PShould    int16
	PRecv      int16
	LastSendTimestamp int16
	Rdelay     uint8
}

func (md *MetrixDataUp) Marshal() []byte {
	data := make([]byte, 22)
	data[0] = UdpMessageExtraTypeMetrix
	binary.BigEndian.PutUint16(data[1:3], uint16(19))
	data[3] = YCKMetrixDataTypeUp
	data[4] = md.Tid
	binary.BigEndian.PutUint32(data[5:9], uint32(md.Bytes))
	binary.BigEndian.PutUint16(data[9:11], uint16(md.Times))
	binary.BigEndian.PutUint32(data[11:15], uint32(md.Bandwidth))
	binary.BigEndian.PutUint16(data[15:17], uint16(md.PShould))
	binary.BigEndian.PutUint16(data[17:19], uint16(md.PRecv))
	binary.BigEndian.PutUint16(data[19:21], uint16(md.LastSendTimestamp))
	data[21] = md.Rdelay

	return data
}

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
	lastTimestampRTT int64
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		stat:          [StatBufferSize]UmsgStat{},
		pos:           0,
		lastTimestamp: time.Now().UnixNano(),
		lastTimestampRTT: time.Now().UnixNano(),
	}

	return metrics
}

func (m *Metrics) Process(msg *Message, timestamp int64) (ok bool, data *MetrixDataUp) {
	var dataUp  *MetrixDataUp
	dataUp = nil

	m.stat[m.pos].paired = false
	m.stat[m.pos].tid = msg.Tid
	m.stat[m.pos].tseq = msg.Tseq
	m.stat[m.pos].bytes = msg.NetTrafficSize()
	currentTimestamp := timestamp
	m.stat[m.pos].timestamp = currentTimestamp

	m.pos++
	if m.pos >= StatBufferSize || (currentTimestamp-m.lastTimestamp) > int64(250 * time.Millisecond) && m.pos > 30 {
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
						deltaTime := m.stat[q].timestamp - u1.timestamp
						accPairs++
						accBytes += uint32(m.stat[q].bytes) //这里的假设是relay自己的下行带宽足够，而计算客户端的上行带宽
						accTimes += deltaTime
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

		bandwidth := -1
		if accPairs > 0 && accTimes > 0 {
			bandwidth = int(8 * int64(accBytes) * int64(time.Second) / int64(accTimes) / 1024)
		}

		logging.Logger.Info(msg.From, " 应收包:", packetShould, " 实收包:", packetRecv, " 重复:", packetDup, " 带宽:", bandwidth, " pairs:", accPairs)

		if packetShould > 0 {
			dataUp = &MetrixDataUp{}
			dataUp.Tid = msg.Tid
			dataUp.Bytes = int32(totalBytes)
			dataUp.Times = int16(totalTime)
			dataUp.Bandwidth = int32(bandwidth)
			dataUp.PShould = int16(packetShould)
			dataUp.PRecv = int16(packetRecv)
			dataUp.LastSendTimestamp = int16(msg.Timestamp)
			dataUp.Rdelay = 0
		}

		//m.pos = 0  //上一批的最后5个，在下一批继续用于计算，在间隙性分批收包的情况下，有助于计算带宽
		reuse := 20
		if reuse < m.pos {
			for i:=0; i<reuse; i++ {
				m.stat[i] = m.stat[m.pos-reuse+i]
				m.stat[i].paired = false
			}
			m.pos = reuse
		}
	}

	if dataUp != nil {
		return true, dataUp
	} else {
	    return false, nil
	}
}
