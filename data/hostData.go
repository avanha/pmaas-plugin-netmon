package data

import "time"

type HostData struct {
	Name                 string
	IpAddress            string
	LastUpdateTime       time.Time
	SnmpStatus           string
	UptimeSeconds        uint64
	PingStatus           string
	PingPacketLoss       float64
	PingRttAverage       time.Duration
	PingRttMin           time.Duration
	PingRttMax           time.Duration
	PingRttStdDev        time.Duration
	NetInterfaceDataList []NetInterfaceData
	Reachability         int
}
