package entities

import (
	"reflect"

	"github.com/avanha/pmaas-spi/tracking"
)

type Host interface {
	tracking.Trackable
	Name() string
}

var HostType = reflect.TypeOf((*Host)(nil)).Elem()
