package common

import "time"

type HostData struct {
	LastUpdateTime time.Time
	UptimeSeconds  uint64
	IfDataList     []IfData
}
