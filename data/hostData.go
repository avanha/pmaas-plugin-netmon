package data

import "time"

type HostData struct {
	Name                 string
	IpAddress            string
	LastUpdateTime       time.Time
	UptimeSeconds        uint64
	NetInterfaceDataList []NetInterfaceData
}
