package common

import "github.com/avanha/pmaas-plugin-netmon/data"

type StatusAndEntities struct {
	//Status     data.PluginStatus
	Hosts []data.HostData
}

type EntityStore interface {
	GetStatusAndEntities() (StatusAndEntities, error)
}
