package netinterface

import (
	"bytes"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/avanha/pmaas-plugin-netmon/config"
	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-plugin-netmon/entities"
	netmonevents "github.com/avanha/pmaas-plugin-netmon/events"
	"github.com/avanha/pmaas-plugin-netmon/internal/common"
	"github.com/avanha/pmaas-spi"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/events"
	"github.com/avanha/pmaas-spi/tracking"

	commonslices "github.com/avanha/pmaas-common/slices"
)

func CreateNetInterface(hostId string,
	id string, trackingConfig tracking.Config, netInterface config.NetInterface) *NetInterface {
	return &NetInterface{
		id:             id,
		hostId:         hostId,
		config:         netInterface,
		trackingConfig: trackingConfig,
		data: data.NetInterfaceData{
			Name: netInterface.TrackingName(),
		},
		onAddressChangeListeners: netInterface.OnAddressChangeListeners(),
	}
}

type NetInterface struct {
	id                                          string
	hostId                                      string
	trackingConfig                              tracking.Config
	config                                      config.NetInterface
	data                                        data.NetInterfaceData
	pmaasEntityId                               string
	hostPmaasEntityId                           string
	stub                                        *stub
	onAddressChangeListeners                    []func(event netmonevents.HostInterfaceAddressChangeEvent)
	onAddressChangeListenersEventReceiverHandle int
}

func (n *NetInterface) Id() string {
	return n.id
}

func (n *NetInterface) TrackingConfig() tracking.Config {
	return n.trackingConfig
}

func (n *NetInterface) Data() tracking.DataSample {
	return tracking.DataSample{
		LastUpdateTime: n.data.LastUpdateTime,
		Data:           n.data,
	}
}

func (n *NetInterface) InterfaceData() data.NetInterfaceData {
	return n.data
}

func (n *NetInterface) PmaasEntityId() string {
	return n.pmaasEntityId
}

func (n *NetInterface) ClearPmaasEntityId() {
	n.pmaasEntityId = ""
}

func (n *NetInterface) SetPmaasEntityId(pmaasEntityId string) {
	if n.pmaasEntityId != "" {
		panic(fmt.Errorf("interface %s already has pmass entity id %s", n.id, n.pmaasEntityId))
	}

	n.pmaasEntityId = pmaasEntityId
}

func (n *NetInterface) HostPmaasEntityId() string {
	return n.hostPmaasEntityId
}

func (n *NetInterface) SetHostPmaasEntityId(pmaasEntityId string) {
	if n.hostPmaasEntityId != "" {
		panic(fmt.Errorf("interface %s already has host pmass entity id %s", n.id, n.hostPmaasEntityId))
	}

	n.hostPmaasEntityId = pmaasEntityId
}

func (n *NetInterface) GetStub(container spi.IPMAASContainer) entities.NetworkInterface {
	if n.stub == nil {
		n.stub = newNetInterfaceStub(
			n.id,
			&spicommon.ThreadSafeEntityWrapper[entities.NetworkInterface]{
				Container: container,
				Entity:    n,
			})
	}

	return n.stub
}

func (n *NetInterface) CloseStubIfPresent() {
	if n.stub != nil {
		n.stub.close()
		n.stub = nil
	}
}

func (n *NetInterface) Update(
	hostData *common.HostData,
	ifData *common.IfData,
	hostEvent *netmonevents.HostEvent,
	events *[]any) {
	hostInterfaceEvent := netmonevents.HostInterfaceEvent{
		HostEvent:    *hostEvent,
		NetInterface: n.pmaasEntityId,
	}

	now := time.Now()
	var elapsedSeconds uint64 = 0

	if !n.data.LastUpdateTime.IsZero() {
		elapsedSeconds = uint64(now.Sub(n.data.LastUpdateTime) / time.Second)
		n.data.CurrentHistoryIndex = stepHistoryIndex(n.data.CurrentHistoryIndex)
	}

	n.data.LastUpdateTime = now

	if ifData.Index != 0 {
		n.data.Index = uint32(ifData.Index)
	}

	if ifData.PhysAddress != "" {
		n.data.PhysAddress = ifData.PhysAddress
	}

	n.updateStatus(ifData, &hostInterfaceEvent, events)
	n.updateIpAddresses(ifData, &hostInterfaceEvent, events)
	n.updateTrafficStats(hostData.UptimeSeconds, elapsedSeconds, ifData, &hostInterfaceEvent, events)
	n.updateErrorStats(ifData, &hostInterfaceEvent, events)
	n.updateDiscardStats(ifData, &hostInterfaceEvent, events)
}

func (n *NetInterface) updateStatus(ifData *common.IfData, hostInterfaceEvent *netmonevents.HostInterfaceEvent, events *[]any) {
	currentStatus := n.data.Status
	newStatus := describeStatus(ifData.OperStatus)

	if newStatus != currentStatus {
		event := netmonevents.HostInterfaceStatusChangeEvent{
			HostInterfaceEvent: *hostInterfaceEvent,
			OldValue:           currentStatus,
			NewValue:           newStatus,
		}
		*events = append(*events, event)
	}
	n.data.Status = newStatus
}

func (n *NetInterface) updateIpAddresses(ifData *common.IfData, hostInterfaceEvent *netmonevents.HostInterfaceEvent, events *[]any) {
	// Sort the addresses to ensure the slice equality check works consistently
	// and that the addresses display consistently.
	slices.SortFunc(ifData.IpAddresses, IpAddressSortFunc)

	currentIpv4Addresses := n.data.IpV4Addresses
	newIpv4Addresses := commonslices.Apply(
		filter(ifData.IpAddresses, func(entry *common.IpMapEntry) bool { return entry.IpVersion == 4 }),
		func(entry *common.IpMapEntry) string { return entry.Address.String() })

	if len(newIpv4Addresses) > 0 && !slices.Equal(currentIpv4Addresses, newIpv4Addresses) {
		n.data.IpV4Addresses = newIpv4Addresses
		n.data.LastIpV4AddressChangeTime = time.Now()
	}

	currentIpAddresses := n.data.IpAddresses

	if len(ifData.IpAddresses) > 0 &&
		!slices.EqualFunc(
			currentIpAddresses,
			ifData.IpAddresses,
			func(a net.IP, b common.IpMapEntry) bool { return a.Equal(b.Address) }) {
		newAddresses := commonslices.Apply(
			ifData.IpAddresses,
			func(entry *common.IpMapEntry) net.IP { return entry.Address })
		event := netmonevents.HostInterfaceAddressChangeEvent{
			HostInterfaceEvent: *hostInterfaceEvent,
			OldValue:           currentIpAddresses,
			NewValue:           newAddresses,
		}
		*events = append(*events, event)
		n.data.IpAddresses = newAddresses
		n.data.LastIpAddressesChangeTime = time.Now()
	}
}

func IpAddressSortFunc(a, b common.IpMapEntry) int {
	if a.IpVersion < b.IpVersion {
		return -1
	}

	if a.Type > b.IpVersion {
		return 1
	}

	return bytes.Compare(a.Address, b.Address)
}

func describeStatus(operStatus int32) string {
	switch operStatus {
	case 1:
		return "Up"
	case 2:
		return "Down"
	default:
		return "Unknown"
	}
}

func (n *NetInterface) updateTrafficStats(
	uptimeSeconds uint64, elapsedSeconds uint64,
	ifData *common.IfData,
	hostInterfaceEvent *netmonevents.HostInterfaceEvent,
	events *[]any) {
	currentBytesIn := n.data.BytesIn
	currentBytesOut := n.data.BytesOut
	currentPacketsIn := n.data.PacketsIn
	currentPacketsOut := n.data.PacketsOut
	newBytesIn := ifData.GetInOctets()
	newBytesOut := ifData.GetOutOctets()
	newPacketsIn := ifData.GetAllInPackets()
	newPacketsOut := ifData.GetAllOutPackets()

	if elapsedSeconds != 0 {
		if uptimeSeconds <= elapsedSeconds {
			// The device restarted, so counters reset
			n.data.BytesInRateHistory[n.data.CurrentHistoryIndex] = newBytesIn / elapsedSeconds
			n.data.BytesOutRateHistory[n.data.CurrentHistoryIndex] = newBytesOut / elapsedSeconds
		} else if newBytesIn < currentBytesIn || currentBytesOut < currentBytesOut {
			// This indicates a rollover.  We need to grab the max value from ifData since it can differ by source:
			// ifTable uses 32-bit vaues, while ifXTable uses 64-bit values.
			n.data.BytesInRateHistory[n.data.CurrentHistoryIndex] =
				(ifData.GetInOctetsMaxValue() - currentBytesIn + newBytesIn) / elapsedSeconds
			n.data.BytesInRateHistory[n.data.CurrentHistoryIndex] =
				(ifData.GetOutOctetsMaxValue() - currentBytesOut + currentBytesOut) / elapsedSeconds
		} else {
			n.data.BytesInRateHistory[n.data.CurrentHistoryIndex] = (newBytesIn - currentBytesIn) / elapsedSeconds
			n.data.BytesOutRateHistory[n.data.CurrentHistoryIndex] = (newBytesOut - currentBytesOut) / elapsedSeconds
		}
	}

	if currentBytesIn != newBytesIn ||
		currentBytesOut != newBytesOut ||
		currentPacketsIn != newPacketsIn ||
		currentPacketsOut != newPacketsOut {
		event := netmonevents.HostInterfaceTrafficStatsChangeEvent{
			HostInterfaceEvent: *hostInterfaceEvent,
			OldBytesIn:         currentBytesIn,
			NewBytesIn:         newBytesIn,
			OldBytesOut:        currentBytesOut,
			NewBytesOut:        newBytesOut,
			OldPacketsIn:       currentPacketsIn,
			NewPacketsIn:       newPacketsIn,
			OldPacketsOut:      currentPacketsOut,
			NewPacketsOut:      newPacketsOut,
		}
		*events = append(*events, event)
		n.data.BytesIn = newBytesIn
		n.data.BytesOut = newBytesOut
		n.data.PacketsIn = newPacketsIn
		n.data.PacketsOut = newPacketsOut
	}
}

func (n *NetInterface) updateErrorStats(ifData *common.IfData, hostInterfaceEvent *netmonevents.HostInterfaceEvent, events *[]any) {
	currentErrorsIn := n.data.ErrorsIn
	currentErrorsOut := n.data.ErrorsOut
	newErrorsIn := ifData.GetInErrors()
	newErrorsOut := ifData.GetOutErrors()

	if currentErrorsIn != newErrorsIn ||
		currentErrorsOut != newErrorsOut {
		event := netmonevents.HostInterfaceErrorStatsChangeEvent{
			HostInterfaceEvent: *hostInterfaceEvent,
			OldErrorsIn:        currentErrorsIn,
			NewErrorsIn:        newErrorsOut,
			OldErrorsOut:       currentErrorsOut,
			NewErrorsOut:       newErrorsOut,
		}
		*events = append(*events, event)
		n.data.ErrorsIn = newErrorsIn
		n.data.ErrorsOut = newErrorsOut
	}
}

func (n *NetInterface) updateDiscardStats(ifData *common.IfData, hostInterfaceEvent *netmonevents.HostInterfaceEvent, events *[]any) {
	currentDiscardsIn := n.data.DiscardsIn
	currentDiscardsOut := n.data.DiscardsOut
	newDiscardsIn := ifData.GetInDiscards()
	newDiscardsOut := ifData.GetOutDiscards()

	if currentDiscardsIn != newDiscardsIn ||
		currentDiscardsOut != newDiscardsOut {
		event := netmonevents.HostInterfaceDiscardStatsChangeEvent{
			HostInterfaceEvent: *hostInterfaceEvent,
			OldDiscardsIn:      currentDiscardsIn,
			NewDiscardsIn:      newDiscardsIn,
			OldDiscardsOut:     currentDiscardsOut,
			NewDiscardsOut:     newDiscardsOut,
		}
		*events = append(*events, event)
		n.data.DiscardsIn = newDiscardsIn
		n.data.DiscardsOut = newDiscardsOut
	}
}

func (n *NetInterface) RegisterConfiguredListeners(container spi.IPMAASContainer) {
	if len(n.onAddressChangeListeners) == 0 {
		return
	}

	hostPmaasEntityId := n.hostPmaasEntityId
	interfacePmaasEntityId := n.pmaasEntityId

	predicate := func(info *events.EventInfo) bool {
		if info.SourceEntityId != hostPmaasEntityId {
			return false
		}

		event, ok := info.Event.(netmonevents.HostInterfaceAddressChangeEvent)

		if !ok {
			return false
		}

		return event.NetInterface == interfacePmaasEntityId
	}

	receiver := func(info *events.EventInfo) error {
		e := info.Event.(netmonevents.HostInterfaceAddressChangeEvent)
		invocations := make([]func(), len(n.onAddressChangeListeners))
		for i, listener := range n.onAddressChangeListeners {
			invocations[i] = func() {
				listener(e)
			}
		}

		err := container.EnqueueOnServerGoRoutine(invocations)

		if err != nil {
			return fmt.Errorf("error enqueuing HostInterfaceAddressChangeEvent listener invocation: %w\n", err)
		}

		return nil
	}

	handle, err := container.RegisterEventReceiver(predicate, receiver)

	if err != nil {
		panic(fmt.Errorf("failed to register event receiver for interface %s: %w", n.id, err))
	}

	n.onAddressChangeListenersEventReceiverHandle = handle
}

func (n *NetInterface) DeregisterConfiguredListeners(container spi.IPMAASContainer) {
	if n.onAddressChangeListenersEventReceiverHandle == 0 {
		return
	}

	err := container.DeregisterEventReceiver(n.onAddressChangeListenersEventReceiverHandle)

	if err == nil {
		n.onAddressChangeListenersEventReceiverHandle = 0
	} else {
		fmt.Printf("Error deregistering receiver for HostInterfaceAddressChangeEvent: %s\n", err)
	}
}

func filter[S any](inputs []S, predicate func(entry *S) bool) []S {
	results := make([]S, 0, len(inputs))

	for i := 0; i < len(inputs); i++ {
		if predicate(&inputs[i]) {
			results = append(results, inputs[i])
		}
	}

	return results
}

func stepHistoryIndex(index uint) uint {
	if index == data.NetInterfaceDataHistorySize-1 {
		return 0
	}

	return index + 1
}
