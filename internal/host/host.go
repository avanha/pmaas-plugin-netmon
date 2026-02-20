package host

import (
	"fmt"
	"iter"
	"maps"

	"github.com/avanha/pmaas-plugin-netmon/config"
	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-plugin-netmon/entities"
	netmonevents "github.com/avanha/pmaas-plugin-netmon/events"
	"github.com/avanha/pmaas-plugin-netmon/internal/common"
	"github.com/avanha/pmaas-plugin-netmon/internal/netinterface"
	spievents "github.com/avanha/pmaas-spi/events"
)

const (
	ReachabilityUnknown = iota
	ReachabilityReachable
	ReachabilityUnreachable
)

type Host struct {
	id            string
	config        config.Host
	netInterfaces map[string]*netinterface.NetInterface
	pmassEntityId string
	data          data.HostData
}

func NewHost(id string, config config.Host) *Host {
	return &Host{
		id:            id,
		config:        config,
		netInterfaces: make(map[string]*netinterface.NetInterface),
		data: data.HostData{
			Name:      config.Name,
			IpAddress: config.IpAddress,
		},
	}
}

func (h *Host) Id() string {
	return h.id
}

func (h *Host) Name() string {
	return h.config.Name
}

func (h *Host) IpAddress() string {
	return h.config.IpAddress
}

func (h *Host) NetInterfaces() iter.Seq2[string, *netinterface.NetInterface] {
	return maps.All(h.netInterfaces)
}

func (h *Host) PmaasEntityId() string {
	return h.pmassEntityId
}

func (h *Host) PingEnabled() bool {
	return h.config.PingEnabled
}

func (h *Host) PingUseIcmp() bool {
	return h.config.PingUseIcmp
}

func (h *Host) PingCount() int {
	return h.config.PingCount
}

func (h *Host) PingTimeoutSeconds() int {
	return h.config.PingTimeoutSeconds
}

func (h *Host) SnmpEnabled() bool {
	return h.config.SnmpEnabled
}

func (h *Host) ClearPmaasEntityId() {
	h.pmassEntityId = ""
}

func (h *Host) SetPmaasEntityId(pmassEntityId string) {
	if h.pmassEntityId != "" {
		panic(fmt.Errorf("host %s already has pmass entity id %s", h.id, h.pmassEntityId))
	}

	h.pmassEntityId = pmassEntityId
}

func (h *Host) AddNetInterface(key string, netInterface *netinterface.NetInterface) {
	h.netInterfaces[key] = netInterface
}

func (h *Host) Update(newData *common.HostData, events *[]any) {
	h.data.LastUpdateTime = newData.LastUpdateTime

	hostEvent := netmonevents.HostEvent{
		EntityEvent: spievents.EntityEvent{
			Id:         h.pmassEntityId,
			EntityType: entities.HostType,
			Name:       h.Name(),
		},
	}

	pingReachability := ReachabilityUnknown
	snmpReachability := ReachabilityUnknown

	if h.PingEnabled() && newData.PingPacketsSent > 0 {
		pingReachability = h.updatePingData(newData, &hostEvent, events)
	}

	if h.SnmpEnabled() {
		snmpReachability = h.updateSnmpData(newData, &hostEvent, events)
	}

	newReachability := calcReachability(pingReachability, snmpReachability)

	if h.data.Reachability != newReachability {
		*events = append(*events, netmonevents.HostReachabilityChangeEvent{
			HostEvent: hostEvent,
			OldValue:  h.data.Reachability,
			NewValue:  newReachability,
		})
		h.data.Reachability = newReachability
	}
}

func (h *Host) updatePingData(newData *common.HostData, hostEvent *netmonevents.HostEvent, events *[]any) int {
	h.data.PingStatus = newData.PingStatus

	if h.data.PingPacketLoss != newData.PingPacketLoss {
		*events = append(*events, netmonevents.HostPingPacketLossChangeEvent{
			HostEvent: *hostEvent,
			OldValue:  h.data.PingPacketLoss,
			NewValue:  newData.PingPacketLoss,
		})
		h.data.PingPacketLoss = newData.PingPacketLoss
	}

	h.data.PingRttAverage = newData.PingRttAvg
	h.data.PingRttMin = newData.PingRttMin
	h.data.PingRttMax = newData.PingRttMax
	h.data.PingRttStdDev = newData.PingRttStdDev

	if newData.PingPacketLoss == 100 {
		return ReachabilityUnreachable
	}

	return ReachabilityReachable
}

func (h *Host) updateSnmpData(newData *common.HostData, hostEvent *netmonevents.HostEvent, events *[]any) int {
	h.data.SnmpStatus = newData.SnmpStatus

	if newData.UptimeSeconds != 0 && h.data.UptimeSeconds != newData.UptimeSeconds {
		*events = append(*events, netmonevents.HostUptimeChangeEvent{
			HostEvent: *hostEvent,
			OldValue:  h.data.UptimeSeconds,
			NewValue:  newData.UptimeSeconds,
		})
		h.data.UptimeSeconds = newData.UptimeSeconds
	}

	for _, ifData := range newData.IfDataList {
		h.updateInterface(newData, &ifData, hostEvent, events)
	}

	if newData.SnmpSuccess {
		return ReachabilityReachable
	}

	return ReachabilityUnreachable
}

func calcReachability(pingReachability, snmpReachability int) int {
	if pingReachability == ReachabilityReachable || snmpReachability == ReachabilityReachable {
		return ReachabilityReachable
	}

	if pingReachability == ReachabilityUnknown && snmpReachability == ReachabilityUnknown {
		return ReachabilityUnknown
	}

	// None were reachable, and at least one status was known to be unreachable, so unreachable it is
	return ReachabilityUnreachable
}

func (h *Host) Data() data.HostData {
	return h.data
}

func (h *Host) updateInterface(hostData *common.HostData, ifData *common.IfData, hostEvent *netmonevents.HostEvent, events *[]any) {
	interfaceInstance := h.findInterface(ifData)

	if interfaceInstance == nil {
		return
	}

	interfaceInstance.Update(hostData, ifData, hostEvent, events)
}

type interfaceIdStrategy func(ifData *common.IfData) (string, string)

var interfaceIdStrategies = []interfaceIdStrategy{
	func(ifData *common.IfData) (string, string) {
		if ifData.Name == "" {
			return "name", ""
		}

		return "name", config.GetInterfaceNameKey(ifData.Name)
	},
	func(ifData *common.IfData) (string, string) {
		if ifData.PhysAddress == "" {
			return "physAddress", ""
		}

		return "physAddress", config.GetInterfacePhysAddressKey(ifData.PhysAddress)
	},
	func(ifData *common.IfData) (string, string) {
		return "ifIndex", config.GetInterfaceIndexKey(ifData.Index)
	},
}

func (h *Host) findInterface(ifData *common.IfData) *netinterface.NetInterface {
	for _, strategy := range interfaceIdStrategies {
		_, key := strategy(ifData)

		if key == "" {
			//fmt.Printf("%T findInterface strategy %s not applicable\n", p, strategyName)
			continue
		}

		iface := h.findInterfaceByKey(key)

		if iface == nil {
			//fmt.Printf("%T findInterface by %s failed\n", p, key)
		} else {
			fmt.Printf("Host [%s]: findInterface %s succeeded\n", h.config.Name, key)
			return iface
		}
	}

	return nil
}

func (h *Host) findInterfaceByKey(key string) *netinterface.NetInterface {
	iface, ok := h.netInterfaces[key]

	if ok {
		return iface
	}

	return nil
}
