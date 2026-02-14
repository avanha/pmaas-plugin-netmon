package monitoring

// Good source for MIB info: https://mibs.observium.org/mib/DISMAN-EVENT-MIB/
const oidSysUptime = ".1.3.6.1.2.1.1.3.0"

// https://mibs.observium.org/mib/IF-MIB/#ifTable
const oidIfTable = ".1.3.6.1.2.1.2.2"
const oidIfTableIfIndex = ".1.3.6.1.2.1.2.2.1.1."
const oidIfTableIfDescr = ".1.3.6.1.2.1.2.2.1.2."
const oidIfTableIfInOctets = ".1.3.6.1.2.1.2.2.1.10."
const oidIfTableIfInUcastPkts = ".1.3.6.1.2.1.2.2.1.11."
const oidIfTableIfOutOctets = ".1.3.6.1.2.1.2.2.1.16."
const oidIfTableIfOutUcastPkts = ".1.3.6.1.2.1.2.2.1.17."
const oidIfTableIfInErrors = ".1.3.6.1.2.1.2.2.1.14."
const oidIfTableIfOutErrors = ".1.3.6.1.2.1.2.2.1.20."
const oidIfTableIfInDiscards = ".1.3.6.1.2.1.2.2.1.15."
const oidIfTableIfOutDiscards = ".1.3.6.1.2.1.2.2.1.21."
const oidIfTableIfMtu = ".1.3.6.1.2.1.2.2.1.4."
const oidIfTableIfSpeed = ".1.3.6.1.2.1.2.2.1.5."
const oidIfTableIfPhysAddress = ".1.3.6.1.2.1.2.2.1.6."
const oidIfTableIfAdminStatus = ".1.3.6.1.2.1.2.2.1.7."
const oidIfTableIfOperStatus = ".1.3.6.1.2.1.2.2.1.8."
const oidIfTableIfLastChange = ".1.3.6.1.2.1.2.2.1.9."

const oidIfXTable = ".1.3.6.1.2.1.31.1.1"
const oidIfXTableIfHCInOctets = ".1.3.6.1.2.1.31.1.1.1.6."
const oidIfXTableIfHCInUcastPkts = ".1.3.6.1.2.1.31.1.1.1.7."
const oidIfXTableIfHCInMulticastPkts = ".1.3.6.1.2.1.31.1.1.1.8."
const oidIfXTableIfHCInBroadcastPkts = ".1.3.6.1.2.1.31.1.1.1.9."
const oidIfXTableIfHCOutOctets = ".1.3.6.1.2.1.31.1.1.1.10."
const oidIfXTableIfHCOutUcastPkts = ".1.3.6.1.2.1.31.1.1.1.11."
const oidIfXTableIfHCOutMulticastPkts = ".1.3.6.1.2.1.31.1.1.1.12."
const oidIfXTableIfHCOutBroadcastPkts = ".1.3.6.1.2.1.31.1.1.1.13."

const oidIpAddrTable = ".1.3.6.1.2.1.4.20"
const oidIpAddrTableIpAddEntAddr = ".1.3.6.1.2.1.4.20.1.1."
const oidIpAddrTableIpAddEntIfIndex = ".1.3.6.1.2.1.4.20.1.2."
const oidIpAddrTableIpAddEntNetMask = ".1.3.6.1.2.1.4.20.1.3."
const oidIpAddrTableIpAddEntBcastAddr = ".1.3.6.1.2.1.4.20.1.4."
const oidIpAddrTableIpAdEntReasmMaxSize = ".1.3.6.1.2.1.4.20.1.5."

// IP-MIB - Mikrotik does not support this, but it does have ipAddrTable (.1.3.6.1.2.1.4.20)
// It also supports the deprecated IPV4-MIB
// https://mibs.observium.org/mib/IP-MIB/#ipAddressTable
// https://www.net-snmp.org/docs/mibs/ip.html#ipAddressTable
const oidIpAddressTable = ".1.3.6.1.2.1.4.34"

const oidIpAddressTableIpAddressIfIndex = ".1.3.6.1.2.1.4.34.1.3." // It's either a .1 or a .2 followed by the length
const oidIpAddressTableIpAddressType = ".1.3.6.1.2.1.4.34.1.4."
const oidIpAddressTableIpAddressPrefix = ".1.3.6.1.2.1.4.34.1.5."
const oidIpAddressTableIpAddressOrigin = ".1.3.6.1.2.1.4.34.1.6."
const oidIpAddressTableIpAddressStatus = ".1.3.6.1.2.1.4.34.1.7."

// 8-11 are timestamps and storage enums that we don't care about

//iso.3.6.1.2.1.4.34.1.3.1.4.10.31.14.255 = INTEGER: 2
//iso.3.6.1.2.1.4.34.1.4.1.4.10.31.14.255 = INTEGER: 3
//iso.3.6.1.2.1.4.34.1.5.1.4.10.31.14.255 = OID: iso.3.6.1.2.1.4.32.1.5.2.1.4.10.31.14.0.24
//iso.3.6.1.2.1.4.34.1.6.1.4.10.31.14.255 = INTEGER: 2
//iso.3.6.1.2.1.4.34.1.7.1.4.10.31.14.255 = INTEGER: 1
//iso.3.6.1.2.1.4.34.1.8.1.4.10.31.14.255 = Timeticks: (0) 0:00:00.00
//iso.3.6.1.2.1.4.34.1.9.1.4.10.31.14.255 = Timeticks: (0) 0:00:00.00
//iso.3.6.1.2.1.4.34.1.10.1.4.10.31.14.255 = INTEGER: 1
//iso.3.6.1.2.1.4.34.1.11.1.4.10.31.14.255 = INTEGER: 2

// .1.3.6.1.2.1.4.34.1.6.1.4.10.0.2.1 = INTEGER: 2
// .1.3.6.1.2.1.4.34.1.6.2.16.38.5.166.1.169.197.6.2.30.105.122.255.254.13.177.225 = INTEGER: 5

const (
	ipAddressTableIpAddressTypeUnicast = iota + 1
	ipAddressTableIpAddressTypeAnycast
	ipAddressTableIpAddressTypeBroadcast
)

const (
	ipAddressTableipAddressOriginOther = iota + 1
	ipAddressTableipAddressOriginManual
	ipAddressTableipAddressOriginUnused1
	ipAddressTableipAddressOriginDhcp
	ipAddressTableipAddressOriginLinklayer
	ipAddressTableipAddressOriginRandom
)

const (
	ValueTypeStringBytes     = iota + 1
	ValueTypeInt32           = 2
	ValueTypeUint32          = 3
	ValueTypeTimeTicks       = 4
	ValueTypePhysicalAddress = 5
	ValueTypeUint64          = 6
	ValueTypeString          = 7
)
