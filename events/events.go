package events

import (
	"net"

	"github.com/avanha/pmaas-spi/events"
)

type HostEvent struct {
	events.EntityEvent
}

type HostUptimeChangeEvent struct {
	HostEvent
	OldValue uint64
	NewValue uint64
}

type HostInterfaceEvent struct {
	HostEvent
	NetInterface string
}

type HostInterfaceStatusChangeEvent struct {
	HostInterfaceEvent
	OldValue string
	NewValue string
}

type HostInterfaceAddressChangeEvent struct {
	HostInterfaceEvent
	OldValue []net.IP
	NewValue []net.IP
}

type HostInterfaceTrafficStatsChangeEvent struct {
	HostInterfaceEvent
	OldBytesIn    uint64
	NewBytesIn    uint64
	OldBytesOut   uint64
	NewBytesOut   uint64
	OldPacketsIn  uint64
	NewPacketsIn  uint64
	OldPacketsOut uint64
	NewPacketsOut uint64
}

type HostInterfaceErrorStatsChangeEvent struct {
	HostInterfaceEvent
	OldErrorsIn  uint64
	NewErrorsIn  uint64
	OldErrorsOut uint64
	NewErrorsOut uint64
}

type HostInterfaceDiscardStatsChangeEvent struct {
	HostInterfaceEvent
	OldDiscardsIn  uint64
	NewDiscardsIn  uint64
	OldDiscardsOut uint64
	NewDiscardsOut uint64
}
