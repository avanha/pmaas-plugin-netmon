package http

import (
	"time"

	"github.com/avanha/pmaas-plugin-netmon/data"
)

type hostWithInterfaces struct {
	data.HostData
	RelativeUptime time.Duration
	Interfaces     []*hostInterfaceWithRenderer
}
