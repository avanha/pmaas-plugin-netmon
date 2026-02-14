package http

import (
	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-spi"
)

type hostInterfaceWithRenderer struct {
	NetInterface data.NetInterfaceData
	Renderer     spi.EntityRenderFunc
}
