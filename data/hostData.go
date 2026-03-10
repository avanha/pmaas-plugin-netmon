package data

import (
	"reflect"
	"time"
)

type HostData struct {
	Name                          string
	IpAddress                     string
	LastUpdateTime                time.Time `track:"always"`
	SnmpStatus                    string
	UptimeSeconds                 uint64 `track:"always"`
	PingStatus                    string
	PingPacketsSent               int
	PingPacketLoss                float64       `track:"always"`
	PingRttAverage                time.Duration `track:"always,dataType=bigint"`
	PingRttMin                    time.Duration `track:"always,dataType=bigint"`
	PingRttMax                    time.Duration `track:"always,dataType=bigint"`
	PingRttStdDev                 time.Duration `track:"always,dataType=bigint"`
	PingUnreachableStartCount     int
	LastPingUnreachableStartTime  time.Time
	LastPingReachableStartTime    time.Time
	LastPingPartialPacketLossTime time.Time
	NetInterfaceDataList          []NetInterfaceData
	Reachability                  int `track:"always,dataType=int"`
	LastReachabilityChangeTime    time.Time
	UnreachableStartCount         int
	LastUnreachableStartTime      time.Time
}

var HostDataType = reflect.TypeOf((*HostData)(nil)).Elem()

func HostDataToInsertArgs(genericDataPointer *any) ([]any, error) {
	data := (*genericDataPointer).(HostData)
	args := []any{
		data.LastUpdateTime,
		data.UptimeSeconds,
		data.PingPacketLoss,
		int64(data.PingRttAverage),
		int64(data.PingRttMin),
		int64(data.PingRttMax),
		int64(data.PingRttStdDev),
		data.Reachability,
	}

	return args, nil
}
