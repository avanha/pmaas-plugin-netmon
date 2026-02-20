package monitoring

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/avanha/pmaas-plugin-netmon/internal/common"
	"github.com/avanha/pmaas-plugin-netmon/internal/host"
	"github.com/gosnmp/gosnmp"
	probing "github.com/prometheus-community/pro-bing"
)

var oids = [...]string{
	oidSysUptime, // Uptime
}

type updateHostFunc func(host *host.Host, hostData common.HostData)

type Task struct {
	ctx                 context.Context
	scanIntervalSeconds int64
	useBulkWalk         bool
	targetAddress       string
	targetName          string
	host                *host.Host
	lastInterfaceCount  int
	updateHostFn        updateHostFunc
}

func CreateTask(ctx context.Context, host *host.Host, updateHostFn updateHostFunc) Task {
	return Task{
		ctx:                 ctx,
		targetAddress:       host.IpAddress(),
		targetName:          host.Name(),
		host:                host,
		updateHostFn:        updateHostFn,
		scanIntervalSeconds: 60,
		useBulkWalk:         true,
	}
}

func (mt *Task) Run() {
	run := mt.randomDelay(mt.scanIntervalSeconds)
	duration := time.Duration(mt.scanIntervalSeconds) * time.Second
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for run {
		mt.scan()
		run = mt.waitForTick(ticker)
	}

	fmt.Printf("monitoring task [%s]: Terminated\n", mt.targetName)
}

func (mt *Task) randomDelay(maxDelay int64) bool {
	maxRandom := maxDelay - 15

	if maxRandom < 15 {
		maxRandom = 15
	}

	// For prod
	delay := 10 + rand.Int64N(maxRandom)

	// For test
	//delay := 1 + rand.Int64N(2)

	fmt.Printf("monitoring task [%s]: Initially pausing for %d seconds\n", mt.targetName, delay)
	return mt.wait(time.Duration(delay) * time.Second)
}

func (mt *Task) scan() {
	fmt.Printf("monitoring task [%s]: Scanning\n", mt.targetName)

	scanStartTime := time.Now()

	data := common.HostData{
		LastUpdateTime: scanStartTime,
	}

	if mt.host.PingEnabled() {
		mt.pingProbe(&data)
	}

	if mt.host.SnmpEnabled() {
		mt.snmpScan(&data)
	}

	// Update the host instance with the retrieved data
	//fmt.Printf("monitoring task [%s]: calling updateHostFn\n", mt.targetName)
	mt.updateHostFn(mt.host, data)
	//fmt.Printf("monitoring task [%s]: finished updateHostFn\n", mt.targetName)
}

func (mt *Task) pingProbe(data *common.HostData) {
	fmt.Printf("monitoring task [%s]: Pinging with %d packets %d second timeout\n",
		mt.targetName, mt.host.PingCount(), mt.host.PingTimeoutSeconds())
	pinger, cancelFn, err := mt.createPinger(mt.targetAddress)

	if err != nil {
		fmt.Printf("monitoring task [%s]: Failed to create pinger: %s\n", mt.targetName, err)
		data.PingStatus = fmt.Sprintf("Unable to ping: %s", err)
		return
	}

	defer cancelFn()
	err = pinger.Run()

	if err != nil {
		fmt.Printf("monitoring task [%s]: Failed to ping: %s\n", mt.targetName, err)
		data.PingStatus = fmt.Sprintf("Unable to ping: %s", err)
		return
	}

	stats := pinger.Statistics()

	if stats.PacketsSent == 0 {
		data.PingStatus = "Cancelled"
		return
	}

	data.PingPacketsSent = stats.PacketsSent

	if stats.PacketLoss >= 100 {
		data.PingStatus = "Timeout"
	} else {
		data.PingStatus = "OK"
	}

	data.PingPacketLoss = stats.PacketLoss
	data.PingRttAvg = stats.AvgRtt
	data.PingRttMin = stats.MinRtt
	data.PingRttMax = stats.MaxRtt
	data.PingRttStdDev = stats.StdDevRtt
}

func (mt *Task) createPinger(targetAddress string) (*probing.Pinger, context.CancelFunc, error) {
	pinger, err := probing.NewPinger(targetAddress)

	if err != nil {
		return nil, nil, err
	}

	pinger.Count = mt.host.PingCount()
	pinger.Timeout = time.Duration(mt.host.PingTimeoutSeconds()) * time.Second

	if mt.host.PingUseIcmp() {
		pinger.SetPrivileged(true)
	}

	// Create a child context that will be marked done either via the task's context or
	// via the cancel function that must be invoked by the caller
	ctx, cancelFn := context.WithCancel(mt.ctx)

	go func() {
		<-ctx.Done()
		// Stop is idempotent.  It can be called when the pinger completes Run naturally,
		// with no ill effect.
		pinger.Stop()
	}()

	return pinger, cancelFn, nil
}

func (mt *Task) snmpScan(data *common.HostData) {
	fmt.Printf("monitoring task [%s]: Retrieving snmp data\n", mt.targetName)
	target := &gosnmp.GoSNMP{
		Context:            mt.ctx,
		Target:             mt.targetAddress,
		Port:               161,
		Transport:          "udp",
		Community:          "public",
		Version:            gosnmp.Version2c,
		Timeout:            time.Duration(2) * time.Second,
		Retries:            3,
		ExponentialTimeout: false,
		MaxOids:            gosnmp.MaxOids,
	}
	scanStartTime := time.Now()

	if err := target.Connect(); err != nil {
		fmt.Printf("monitoring task [%s]: Unable to connect: %v\n", mt.targetName, err)
		data.SnmpStatus = fmt.Sprintf("Unable to connect: %v", err)
		return
	}

	defer func() {
		if err := target.Close(); err != nil {
			fmt.Printf("monitoring task [%s]: Error closing connection: %v\n", mt.targetName, err)
		}
	}()

	uptimeSuccess := mt.getUptime(target, data)
	ifTableSuccess := mt.getIfTable(target, data, mt.lastInterfaceCount)

	if ifTableSuccess {
		mt.getIfXTable(target, data)
		ipAddressTableSuccess := mt.getIpAddressTable(target, data)

		if !ipAddressTableSuccess {
			// There's no need to get the deprecated IPv4-MIB::IpAddrTable if the host supports the
			// newer IP-MIB::IpAddressTable
			mt.getIpAddrTable(target, data)
		}
	}

	fmt.Printf("monitoring task [%s]: snmp walk completed in %v\n", mt.targetName, time.Since(scanStartTime))

	if uptimeSuccess || ifTableSuccess {
		data.SnmpStatus = "OK"
		data.SnmpSuccess = true
	} else {
		data.SnmpStatus = "Failed to retrieve any data"
	}

	// Store the count of interfaces so we have it for next time
	mt.lastInterfaceCount = len(data.IfDataList)
}

func (mt *Task) getUptime(target *gosnmp.GoSNMP, data *common.HostData) bool {
	result, err := target.Get(oids[:])

	if err != nil {
		fmt.Printf("monitoring task [%s]: Error retrieving values: %v\n", mt.targetName, err)
		return false
	}

	//fmt.Printf("monitoring task [%s]: retrieved values: %v\n", mt.targetName, result)

	var hostUptimeSeconds uint64

	for _, variable := range result.Variables {
		switch variable.Name {
		case oidSysUptime:
			value := gosnmp.ToBigInt(variable.Value)

			if value.IsUint64() {
				hostUptimeSeconds = value.Uint64() / 100
			}

		default:
			printSNMPData(variable)
		}
	}

	data.UptimeSeconds = hostUptimeSeconds

	return true
}

type ifDataSetter[T any] func(T, *common.IfData)

type parserSpec struct {
	valueType    int
	stringSetter ifDataSetter[string]
	int32Setter  ifDataSetter[int32]
	uint32Setter ifDataSetter[uint32]
	uint64Setter ifDataSetter[uint64]
}

var ifTableParserMap = map[string]*parserSpec{
	oidIfTableIfDescr: {valueType: ValueTypeStringBytes, stringSetter: func(value string, data *common.IfData) {
		data.Name = value
	}},
	oidIfTableIfInOctets: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.InOctets = value
	}},
	oidIfTableIfOutOctets: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.OutOctets = value
	}},
	oidIfTableIfInUcastPkts: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.InUcastPkts = value
	}},
	oidIfTableIfOutUcastPkts: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.OutUcastPkts = value
	}},
	oidIfTableIfInErrors: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.InErrors = value
	}},
	oidIfTableIfOutErrors: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.OutErrors = value
	}},
	oidIfTableIfInDiscards: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.InDiscards = value
	}},
	oidIfTableIfOutDiscards: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.OutDiscards = value
	}},
	oidIfTableIfMtu: {valueType: ValueTypeInt32, int32Setter: func(value int32, data *common.IfData) {
		data.Mtu = value
	}},
	oidIfTableIfSpeed: {valueType: ValueTypeUint32, uint32Setter: func(value uint32, data *common.IfData) {
		data.Speed = value
	}},
	oidIfTableIfPhysAddress: {valueType: ValueTypePhysicalAddress, stringSetter: func(value string, data *common.IfData) {
		data.PhysAddress = value
	}},
	oidIfTableIfAdminStatus: {valueType: ValueTypeInt32, int32Setter: func(value int32, data *common.IfData) {
		data.AdminStatus = value
	}},
	oidIfTableIfOperStatus: {valueType: ValueTypeInt32, int32Setter: func(value int32, data *common.IfData) {
		data.OperStatus = value
	}},
	oidIfTableIfLastChange: {valueType: ValueTypeTimeTicks, uint32Setter: func(value uint32, data *common.IfData) {
		data.LastChangeSeconds = value
	}},
}

var ifXTableParserMap = map[string]*parserSpec{
	oidIfXTableIfHCInOctets: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCInOctets = value
	}},
	oidIfXTableIfHCInUcastPkts: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCInUcastPkts = value
	}},
	oidIfXTableIfHCInMulticastPkts: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCInMulticastPkts = value
	}},
	oidIfXTableIfHCInBroadcastPkts: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCInBroadcastPkts = value
	}},
	oidIfXTableIfHCOutOctets: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCOutOctets = value
	}},
	oidIfXTableIfHCOutUcastPkts: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCOutUcastPkts = value
	}},
	oidIfXTableIfHCOutMulticastPkts: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCOutMulticastPkts = value
	}},
	oidIfXTableIfHCOutBroadcastPkts: {valueType: ValueTypeUint64, uint64Setter: func(value uint64, data *common.IfData) {
		data.HCOutBroadcastPkts = value
	}},
}

type ipAddressMapEntrySetter[T any] func(T, *common.IpMapEntry)

type ipAddressTableParserSpec struct {
	valueType    int
	int32Setter  ipAddressMapEntrySetter[int32]
	stringSetter ipAddressMapEntrySetter[string]
}

var ipAddressTableParserMap = map[string]*ipAddressTableParserSpec{
	oidIpAddressTableIpAddressIfIndex: {
		valueType: ValueTypeInt32, int32Setter: func(value int32, data *common.IpMapEntry) {
			data.IfIndex = value
		},
	},
	oidIpAddressTableIpAddressOrigin: {
		valueType: ValueTypeInt32, int32Setter: func(value int32, data *common.IpMapEntry) {
			data.Origin = value
		},
	},
	oidIpAddressTableIpAddressType: {
		valueType: ValueTypeInt32, int32Setter: func(value int32, data *common.IpMapEntry) {
			data.Type = value
		},
	},
	oidIpAddressTableIpAddressPrefix: {
		valueType: ValueTypeString, stringSetter: func(value string, data *common.IpMapEntry) {
			data.PrefixTableIndex = value
		},
	},
	oidIpAddressTableIpAddressStatus: {
		valueType: ValueTypeInt32, int32Setter: func(value int32, data *common.IpMapEntry) {
			data.Status = value
		},
	},
}

var ipV4AddrTableParserMap = map[string]*ipAddressTableParserSpec{
	oidIpAddrTableIpAddEntAddr: {
		valueType: ValueTypeString,
		stringSetter: func(value string, data *common.IpMapEntry) {
			address := net.ParseIP(value)

			if address == nil {
				return
			}

			address = address.To4()

			if address == nil {
				return
			}

			data.IpVersion = 4
			data.Address = address
		},
	},
	oidIpAddrTableIpAddEntIfIndex: {valueType: ValueTypeInt32,
		int32Setter: func(value int32, data *common.IpMapEntry) {
			data.IfIndex = value
		},
	},
	oidIpAddrTableIpAddEntNetMask: {valueType: ValueTypeString,
		stringSetter: func(value string, data *common.IpMapEntry) {
			data.NetMask = value
		},
	},
	oidIpAddrTableIpAddEntBcastAddr: {valueType: ValueTypeInt32,
		int32Setter: func(value int32, data *common.IpMapEntry) {
			data.BcastAddress = value
		},
	},
	oidIpAddrTableIpAdEntReasmMaxSize: {
		valueType: ValueTypeInt32,
		int32Setter: func(value int32, data *common.IpMapEntry) {
			data.ReasmMaxSize = value
		}},
}

func (mt *Task) getIfTable(target *gosnmp.GoSNMP, data *common.HostData, previousInterfaceCount int) bool {
	// Create a slice with entries, not just capacity, we'll write to it by interface index
	ifData := make([]common.IfData, max(1, previousInterfaceCount))
	dataUnitCount := 0

	var walkFn = func(dataUnit gosnmp.SnmpPDU) error {
		dataUnitCount++
		return mt.processIfTableData(dataUnit, &ifData)
	}

	triedBulkWalk := mt.useBulkWalk

	// Not all devices support bulk walks, so we use a single walk instead, if needed
	if mt.useBulkWalk {
		err := target.BulkWalk(oidIfTable, walkFn)

		if err == nil {
			data.IfDataList = ifData
			return false
		}

		fmt.Printf("monitoring task [%s]: Error retrieving ifTable via BulkWalk: %v\n", mt.targetName, err)
		mt.useBulkWalk = false
	}

	// Recreate ifTable to avoid any partial data from an incomplete bulk walk
	ifData = make([]common.IfData, 0, len(ifData))

	walkStartTime := time.Time{}

	if triedBulkWalk {
		fmt.Printf("monitoring task [%s]: Retrieving ifTable via Walk\n", mt.targetName)
		walkStartTime = time.Now()
	}

	err := target.Walk(oidIfTable, walkFn)

	if err == nil {
		if triedBulkWalk {
			fmt.Printf("monitoring task [%s]: Successfully retrieved ifTable via Walk in %v\n", mt.targetName, time.Since(walkStartTime))
		}
	} else {
		fmt.Printf("monitoring task [%s]: Error retrieving ifTable via Walk: %v\n", mt.targetName, err)
		return false
	}

	data.IfDataList = ifData

	return dataUnitCount > 0
}

func (mt *Task) getIfXTable(target *gosnmp.GoSNMP, data *common.HostData) bool {
	var walkFn = func(dataUnit gosnmp.SnmpPDU) error {
		return mt.processIfXTableData(dataUnit, data.IfDataList)
	}

	var err error = nil

	if mt.useBulkWalk {
		err = target.BulkWalk(oidIfXTable, walkFn)
	} else {
		err = target.Walk(oidIfXTable, walkFn)
	}

	if err != nil {
		fmt.Printf("monitoring task [%s]: Error retrieving ifXTable: %v\n", mt.targetName, err)
		return false
	}

	return true
}

func (mt *Task) getIpAddressTable(target *gosnmp.GoSNMP, data *common.HostData) bool {
	var ipTable = make(map[string]*common.IpMapEntry)

	var walkFn = func(dataUnit gosnmp.SnmpPDU) error {
		return mt.processIpAddressTableData(dataUnit, ipTable)
	}

	var err error = nil

	if mt.useBulkWalk {
		err = target.BulkWalk(oidIpAddressTable, walkFn)
	} else {
		err = target.Walk(oidIpAddressTable, walkFn)
	}

	if err != nil {
		fmt.Printf("monitoring task [%s]: Error retrieving ipAddressTable: %v\n", mt.targetName, err)
		return false
	}

	// Integrate ipMapEntry instances onto the referenced interfaces
	for key, ipMapEntry := range ipTable {
		if ipMapEntry.IfIndex > 0 && ipMapEntry.IfIndex <= int32(len(data.IfDataList)) {
			err := populateIpAddressAndVersion(key, ipMapEntry)

			if err == nil && (ipMapEntry.IpVersion == 4 || ipMapEntry.IpVersion == 6) {
				if ipMapEntry.Type == ipAddressTableIpAddressTypeBroadcast {
					// Ignore broadcast addresses
				} else {
					data.IfDataList[ipMapEntry.IfIndex-1].IpAddresses =
						append(data.IfDataList[ipMapEntry.IfIndex-1].IpAddresses, *ipMapEntry)
				}
			} else {
				fmt.Printf("monitoring task [%s]: invalid ipAddressTable entry %s: %v\n",
					mt.targetName, key, err)
			}
		} else {
			fmt.Printf("monitoring task [%s]: ipMapEntry.IfIndex (%d) is out of range (%d)\n",
				mt.targetName, ipMapEntry.IfIndex, len(data.IfDataList))
		}
	}

	return len(ipTable) > 0
}

func populateIpAddressAndVersion(key string, ipMapEntry *common.IpMapEntry) error {
	keyParts := strings.Split(key, ".")

	if len(keyParts) < 6 {
		return fmt.Errorf("invalid ipAddressTable key %s", key)
	}

	if keyParts[0] == "1" && keyParts[1] == "4" && len(keyParts) == 6 {
		address, err := buildIpAddress(keyParts[2:])

		if err != nil {
			return fmt.Errorf("invalid IPv4 address: %s: %w", key, err)
		}

		ipMapEntry.IpVersion = 4
		ipMapEntry.Address = address
	} else if keyParts[0] == "2" && keyParts[1] == "16" && len(keyParts) == 18 {
		address, err := buildIpAddress(keyParts[2:])

		if err != nil {
			return fmt.Errorf("invalid IPv6 address: %s: %w", key, err)
		}

		ipMapEntry.IpVersion = 6
		ipMapEntry.Address = address
	} else {
		return fmt.Errorf("invalid ipAddressTable key %s", key)
	}

	return nil
}

func buildIpAddress(keyParts []string) (net.IP, error) {
	bytes := make([]byte, len(keyParts))

	for i, value := range keyParts {
		b, err := strconv.Atoi(value)

		if err != nil {
			return nil, fmt.Errorf("invalid string in IP address OID: %w", err)
		}

		bytes[i] = byte(b)
	}

	ip := net.IP(bytes)

	return ip, nil
}

func (mt *Task) getIpAddrTable(target *gosnmp.GoSNMP, data *common.HostData) bool {
	var ipTable = make(map[string]*common.IpMapEntry)

	var walkFn = func(dataUnit gosnmp.SnmpPDU) error {
		return mt.processIpAddrTableData(dataUnit, ipTable)
	}

	var err error = nil

	if mt.useBulkWalk {
		err = target.BulkWalk(oidIpAddrTable, walkFn)
	} else {
		err = target.Walk(oidIpAddrTable, walkFn)
	}

	if err != nil {
		fmt.Printf("monitoring task [%s]: Error retrieving ipAddrTable: %v\n", mt.targetName, err)
		return false
	}

	// Integrate ipMapEntry instances onto the referenced interfaces
	for _, ipMapEntry := range ipTable {
		if ipMapEntry.IfIndex > 0 && ipMapEntry.IfIndex <= int32(len(data.IfDataList)) {
			data.IfDataList[ipMapEntry.IfIndex-1].IpAddresses =
				append(data.IfDataList[ipMapEntry.IfIndex-1].IpAddresses, *ipMapEntry)
		} else {
			fmt.Printf("monitoring task [%s]: ipMapEntry.IfIndex (%d) is out of range (%d)\n",
				mt.targetName, ipMapEntry.IfIndex, len(data.IfDataList))
		}
	}

	return true
}

func (mt *Task) processIpAddressTableData(
	dataUnit gosnmp.SnmpPDU, ipMap map[string]*common.IpMapEntry) error {
	for oid, spec := range ipAddressTableParserMap {
		if strings.HasPrefix(dataUnit.Name, oid) {
			id := parseIpIdentifier(oid, dataUnit.Name)

			mapEntry, ok := ipMap[id]

			if !ok {
				mapEntry = &common.IpMapEntry{}
				ipMap[id] = mapEntry
			}

			switch spec.valueType {
			case ValueTypeInt32:
				value, ok := parseInt32Value(dataUnit)
				if ok {
					spec.int32Setter(value, mapEntry)
				}
				break
			case ValueTypeString:
				value, ok := parseStringValue(dataUnit)
				if ok {
					spec.stringSetter(value, mapEntry)
				}
				break
			default:
				return fmt.Errorf("unsupported value type %d for oid %s", spec.valueType, oid)
			}
			break
		}
	}

	return nil
}

func (mt *Task) processIpAddrTableData(
	dataUnit gosnmp.SnmpPDU, ipMap map[string]*common.IpMapEntry) error {
	for oid, spec := range ipV4AddrTableParserMap {
		if strings.HasPrefix(dataUnit.Name, oid) {
			id := parseIpIdentifier(oid, dataUnit.Name)

			mapEntry, ok := ipMap[id]

			if !ok {
				mapEntry = &common.IpMapEntry{}
				ipMap[id] = mapEntry
			}

			switch spec.valueType {
			case ValueTypeString:
				value, ok := parseStringValue(dataUnit)
				if ok {
					spec.stringSetter(value, mapEntry)
				}
				break
			case ValueTypeInt32:
				value, ok := parseInt32Value(dataUnit)
				if ok {
					spec.int32Setter(value, mapEntry)
				}
				break
			default:
				return fmt.Errorf("unsupported value type %d for oid %s", spec.valueType, oid)
			}
			break
		}
	}

	return nil
}

func parseIpIdentifier(oidPrefix string, oid string) string {
	return oid[len(oidPrefix):]
}

func (mt *Task) processIfTableData(dataUnit gosnmp.SnmpPDU, interfaces *[]common.IfData) error {
	//printSNMPData(dataUnit)
	if strings.HasPrefix(dataUnit.Name, oidIfTableIfIndex) {
		var index = int32(gosnmp.ToBigInt(dataUnit.Value).Int64())
		// Sanity check: Don't allow more than 1,000 interfaces
		if index > 1000 {
			return fmt.Errorf("interface index %d is out of range", index)
		}

		var currentCount = int32(len(*interfaces))

		if index > currentCount {
			if index-currentCount == 1 {
				*interfaces = append(*interfaces, common.IfData{})
			} else {
				var additional = make([]common.IfData, index-currentCount)
				*interfaces = slices.Concat(*interfaces, additional)
			}
		}

		(*interfaces)[index-1].Index = index

		return nil
	}

	mt.processIfTableDetail("IfTable", ifTableParserMap, dataUnit, *interfaces)

	return nil
}

func (mt *Task) processIfXTableData(dataUnit gosnmp.SnmpPDU, interfaces []common.IfData) error {
	mt.processIfTableDetail("IfXTable", ifXTableParserMap, dataUnit, interfaces)

	return nil
}

func (mt *Task) processIfTableDetail(
	tableName string,
	parserMap map[string]*parserSpec,
	dataUnit gosnmp.SnmpPDU,
	interfaces []common.IfData) {
	interfaceCount := int32(len(interfaces))

	for oid, spec := range parserMap {
		if strings.HasPrefix(dataUnit.Name, oid) {
			index, ok := mt.parseIfIndex(oid, dataUnit.Name)
			if ok {
				offset := index - 1

				if offset >= interfaceCount {
					fmt.Printf(
						"monitoring task [%s]: Interface index %d (array offset %d) in %s on oid %s is out of range (0-%d)\n",
						mt.targetName, index, offset, tableName, oid, interfaceCount-1)
					continue
				}
				switch spec.valueType {
				case ValueTypeString:
					value, ok := parseStringValue(dataUnit)
					if ok {
						spec.stringSetter(value, &interfaces[offset])
					}
					break
				case ValueTypeStringBytes:
					value, ok := parseStringBytesValue(dataUnit)
					if ok {
						spec.stringSetter(value, &interfaces[offset])
					}
					break
				case ValueTypeInt32:
					value, ok := parseInt32Value(dataUnit)
					if ok {
						spec.int32Setter(value, &interfaces[offset])
					}
					break
				case ValueTypeUint32:
					value, ok := parseUint32Value(dataUnit)
					if ok {
						spec.uint32Setter(value, &interfaces[offset])
					}
					break
				case ValueTypeUint64:
					value, ok := parseUint64Value(dataUnit)
					if ok {
						spec.uint64Setter(value, &interfaces[offset])
					}
					break
				case ValueTypeTimeTicks:
					value, ok := parseTimeTicksValue(dataUnit)
					if ok {
						spec.uint32Setter(value, &interfaces[offset])
					}
					break
				case ValueTypePhysicalAddress:
					value, ok := parsePhysicalAddressValue(dataUnit)
					if ok {
						spec.stringSetter(value, &interfaces[offset])
					}
					break
				}
			}
			break
		}
	}
}

func (mt *Task) parseIfIndex(oidPrefix string, oid string) (int32, bool) {
	index, err := strconv.ParseInt(oid[len(oidPrefix):], 10, 32)

	if err != nil {
		fmt.Printf("monitoring task [%s]: Unable to parse interface index from \"%s\"\n", mt.targetName, oid)
		return -1, false
	}

	return int32(index), true
}
func parseStringValue(dataUnit gosnmp.SnmpPDU) (string, bool) {
	value, ok := dataUnit.Value.(string)

	if !ok {
		fmt.Printf("Unable to cast %T value to string for oid \"%s\"\n", dataUnit.Value, dataUnit.Name)
		return "", false
	}

	return value, true
}

func parseStringBytesValue(dataUnit gosnmp.SnmpPDU) (string, bool) {
	value, ok := dataUnit.Value.([]byte)

	if !ok {
		fmt.Printf("Unable to cast %T value to []byte for oid \"%s\"\n", dataUnit.Value, dataUnit.Name)
		return "", false
	}

	return string(value), true
}

func parsePhysicalAddressValue(dataUnit gosnmp.SnmpPDU) (string, bool) {
	value, ok := dataUnit.Value.([]byte)

	if !ok {
		fmt.Printf("Unable to cast %T value to []byte for oid \"%s\"\n", dataUnit.Value, dataUnit.Name)
		return "", false
	}

	return net.HardwareAddr(value).String(), true
}

func parseInt32Value(dataUnit gosnmp.SnmpPDU) (int32, bool) {
	value, ok := dataUnit.Value.(int)

	if !ok {
		fmt.Printf("Unable to cast %T value \"%v\" to int for oid \"%s\"\n",
			dataUnit.Value, dataUnit.Value, dataUnit.Name)
		return 0, false
	}

	return int32(value), true
}

func parseUint32Value(dataUnit gosnmp.SnmpPDU) (uint32, bool) {
	value, ok := dataUnit.Value.(uint)

	if !ok {
		fmt.Printf("Unable to cast %T value to uint for oid \"%s\"\n", dataUnit.Value, dataUnit.Name)
		return 0, false
	}

	return uint32(value), true
}

func parseUint64Value(dataUnit gosnmp.SnmpPDU) (uint64, bool) {
	value, ok := dataUnit.Value.(uint64)

	if !ok {
		fmt.Printf("Unable to cast %T value to uint64 for oid \"%s\"\n", dataUnit.Value, dataUnit.Name)
		return 0, false
	}

	return value, true
}

func parseTimeTicksValue(dataUnit gosnmp.SnmpPDU) (uint32, bool) {
	value, ok := dataUnit.Value.(uint32)

	if !ok {
		fmt.Printf("Unable to cast %T value to uint32 for oid \"%s\"\n", dataUnit.Value, dataUnit.Name)
		return 0, false
	}

	// TimeTicks is in 1/100ths of a second.
	return value / 100, true
}

func printSNMPData(dataUnit gosnmp.SnmpPDU) {
	fmt.Printf("oid: %s ", dataUnit.Name)
	switch dataUnit.Type {
	case gosnmp.OctetString:
		bytes := dataUnit.Value.([]byte)
		fmt.Printf("string: %s\n", string(bytes))
	default:
		// ... or often you're just interested in numeric values.
		// ToBigInt() will return the Value as a BigInt, for plugging
		// into your calculations.
		fmt.Printf("number: %d\n", gosnmp.ToBigInt(dataUnit.Value))
	}
}

func (mt *Task) wait(duration time.Duration) bool {
	timer := time.NewTimer(duration)

	for {
		select {
		case <-mt.ctx.Done():
			timer.Stop()
			return false

		case <-timer.C:
			return true
		}
	}
}

func (mt *Task) waitForTick(ticker *time.Ticker) bool {
	for {
		select {
		case <-mt.ctx.Done():
			return false
		case <-ticker.C:
			return true
		}
	}
}
