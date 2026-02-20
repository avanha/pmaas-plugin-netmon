package config

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/avanha/pmaas-plugin-netmon/events"
)

type Host struct {
	Name               string
	IpAddress          string
	PingEnabled        bool
	PingTimeoutSeconds int
	PingCount          int
	PingUseIcmp        bool
	SnmpEnabled        bool
	NetInterfaces      map[string]*NetInterface
}

func (h *Host) AddNetInterfaceByName(name string) *NetInterface {
	netInterface := &NetInterface{Name: name, IdentificationMode: InterfaceByName}
	h.NetInterfaces[GetInterfaceNameKey(name)] = netInterface

	return netInterface
}

func (h *Host) AddNetInterfaceByIndex(index int32) *NetInterface {
	netInterface := &NetInterface{Index: index, IdentificationMode: InterfaceByIndex}
	h.NetInterfaces[GetInterfaceIndexKey(index)] = netInterface

	return netInterface
}

func (h *Host) AddNetInterfaceByPhysicalAddress(physAddress string) *NetInterface {
	netInterface := &NetInterface{PhysAddress: physAddress, IdentificationMode: InterfaceByPhysAddress}
	h.NetInterfaces[GetInterfacePhysAddressKey(physAddress)] = netInterface

	return netInterface
}

const InterfaceByIndex = 1
const InterfaceByName = 2
const InterfaceByPhysAddress = 3

type NetInterface struct {
	Index                    int32
	Name                     string
	PhysAddress              string
	IdentificationMode       int
	onAddressChangeListeners []func(event events.HostInterfaceAddressChangeEvent)
}

func (i *NetInterface) TrackingName() string {
	switch i.IdentificationMode {
	case InterfaceByIndex:
		return strconv.Itoa(int(i.Index))
	case InterfaceByName:
		return i.Name
	case InterfaceByPhysAddress:
		return i.PhysAddress
	default:
		panic(fmt.Errorf("unknown interface identification mode: %v", i.IdentificationMode))
	}
}

func (i *NetInterface) AddOnIpAddressChangeListener(eventListener func(event events.HostInterfaceAddressChangeEvent)) {
	if i.onAddressChangeListeners == nil {
		i.onAddressChangeListeners = make([]func(event events.HostInterfaceAddressChangeEvent), 1)
		i.onAddressChangeListeners[0] = eventListener
	} else {
		i.onAddressChangeListeners = append(i.onAddressChangeListeners, eventListener)
	}
}

func (i *NetInterface) OnAddressChangeListeners() []func(event events.HostInterfaceAddressChangeEvent) {
	return slices.Clone(i.onAddressChangeListeners)
}

func GetInterfaceNameKey(name string) string {
	return "name:" + name
}

func GetInterfacePhysAddressKey(physAddress string) string {
	return "phys:" + physAddress
}

func GetInterfaceIndexKey(index int32) string {
	return "index:" + strconv.Itoa(int(index))
}
