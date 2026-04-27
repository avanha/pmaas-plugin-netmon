package entities

import (
	"reflect"

	"github.com/avanha/pmaas-spi/tracking"
)

type Host interface {
	tracking.HistoryAwareTrackable
	Name() string
}

var HostType = reflect.TypeOf((*Host)(nil)).Elem()
