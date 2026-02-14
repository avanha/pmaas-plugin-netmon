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

	if newData.UptimeSeconds != 0 && h.data.UptimeSeconds != newData.UptimeSeconds {
		*events = append(*events, netmonevents.HostUptimeChangeEvent{
			HostEvent: hostEvent,
			OldValue:  h.data.UptimeSeconds,
			NewValue:  newData.UptimeSeconds,
		})
		h.data.UptimeSeconds = newData.UptimeSeconds
	}

	for _, ifData := range newData.IfDataList {
		h.updateInterface(newData, &ifData, &hostEvent, events)
	}
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
