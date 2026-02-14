package entities

import "reflect"

type Host interface {
	Name() string
}

var HostType = reflect.TypeOf((*Host)(nil)).Elem()
