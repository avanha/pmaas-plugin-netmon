package entities

import (
	"reflect"

	"github.com/avanha/pmaas-spi/tracking"
)

type NetworkInterface interface {
	tracking.Trackable
}

var NetworkInterfaceType = reflect.TypeOf((*NetworkInterface)(nil)).Elem()
