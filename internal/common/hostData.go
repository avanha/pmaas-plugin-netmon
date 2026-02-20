package common

import "time"

type HostData struct {
	LastUpdateTime  time.Time
	SnmpSuccess     bool
	SnmpStatus      string
	UptimeSeconds   uint64
	IfDataList      []IfData
	PingStatus      string
	PingPacketsSent int
	PingPacketLoss  float64
	PingRttAvg      time.Duration
	PingRttMin      time.Duration
	PingRttMax      time.Duration
	PingRttStdDev   time.Duration
}
