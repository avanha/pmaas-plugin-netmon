package config

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/avanha/pmaas-plugin-netmon/events"
	"github.com/avanha/pmaas-spi/alert"
)

// Prober says whether a host can be reached, in place of pinging it and asking it over SNMP. It is for hosts that are
// not on a network: a fake one, for testing or a demonstration. Reachable is asked once for each poll, on the monitoring
// goroutine, so it must be safe to call from there.
type Prober interface {
	Reachable(ctx context.Context) bool
}

type Host struct {
	Name               string
	IpAddress          string
	PingEnabled        bool
	PingTimeoutSeconds int
	PingCount          int
	PingUseIcmp        bool
	SnmpEnabled        bool
	NetInterfaces      map[string]*NetInterface

	// Prober, if set, is asked whether the host is reachable at each poll, instead of the host being pinged and asked over
	// SNMP. Interfaces are not looked at.
	Prober Prober

	alertAfter    time.Duration
	alertSeverity alert.SeverityFunc
	alertNeeds    *Host
}

// UseProber has the plugin ask the prober whether the host is reachable at each poll, instead of pinging it and asking it over
// SNMP: for a host that isn't on a network, such as a fake one.
func (h *Host) UseProber(prober Prober) *Host {
	h.Prober = prober

	return h
}

// AlertWhenUnreachable has the plugin raise an alert once the host has been unreachable for at least after, and clear it when
// the host is reachable again. The host is looked at once a minute, so the alert comes at the first look after the time has
// passed. severity says how severe the alert is, and is asked at each look, so that the alert can escalate or ease as the
// day goes on (see alert.ByTimeOfDay, and automations.SeverityBetween in pmaas-automations for the sun). With no severity it is
// critical. It panics if after is not positive.
func (h *Host) AlertWhenUnreachable(after time.Duration, severity alert.SeverityFunc) *Host {
	if after <= 0 {
		panic(fmt.Sprintf("netmon: host %q: the time to be unreachable before alerting must be positive, not %v", h.Name, after))
	}

	if severity == nil {
		severity = alert.Always(alert.SeverityCritical)
	}

	h.alertAfter, h.alertSeverity = after, severity

	return h
}

// AlertOnlyWhileReachable holds the host's alert back while another host, which has to be one of the plugin's, is not known
// to be reachable: the other being down is the problem, and its own alert is the one to see, not one for everything
// behind it. A router and the hosts out on the internet beyond it, say.
//
// While the other host is unreachable, or has not been polled yet, the alert is neither raised nor cleared. When it is
// reachable again the time the host has to be unreachable is counted from then, if that is later than when the host went
// down, so a host that was only unreachable because of the other gets its full time to answer before it is reported. It
// panics if there is no other host, or it is this one.
func (h *Host) AlertOnlyWhileReachable(other *Host) *Host {
	if other == nil || other == h {
		panic(fmt.Sprintf("netmon: host %q: the host its alert depends on must be another host", h.Name))
	}

	h.alertNeeds = other

	return h
}

// AlertDependency is the host whose being reachable the alert waits for, or nil if it doesn't.
func (h *Host) AlertDependency() *Host {
	return h.alertNeeds
}

// UnreachableAlert is how long the host has to be unreachable before an alert is raised, and how severe it is, or false if no
// alert was asked for.
func (h *Host) UnreachableAlert() (time.Duration, alert.SeverityFunc, bool) {
	return h.alertAfter, h.alertSeverity, h.alertAfter > 0
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
