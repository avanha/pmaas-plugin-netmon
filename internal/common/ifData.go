package common

import (
	"math"
	"net"
)

type IpMapEntry struct {
	IpVersion        int32
	Address          net.IP
	IfIndex          int32
	Status           int32
	Origin           int32
	Type             int32
	PrefixTableIndex string
	ReasmMaxSize     int32
	BcastAddress     int32
	NetMask          string
}

type IfData struct {
	Index              int32
	Name               string
	InOctets           uint32
	HCInOctets         uint64
	InUcastPkts        uint32
	HCInUcastPkts      uint64
	HCInMulticastPkts  uint64
	HCInBroadcastPkts  uint64
	OutOctets          uint32
	HCOutOctets        uint64
	OutUcastPkts       uint32
	HCOutUcastPkts     uint64
	HCOutMulticastPkts uint64
	HCOutBroadcastPkts uint64
	InErrors           uint32
	OutErrors          uint32
	InDiscards         uint32
	OutDiscards        uint32
	Mtu                int32
	Speed              uint32
	PhysAddress        string
	AdminStatus        int32
	OperStatus         int32
	LastChangeSeconds  uint32
	IpAddresses        []IpMapEntry
}

func (ifd *IfData) GetInOctets() uint64 {
	if ifd.HCInOctets != 0 {
		return ifd.HCInOctets
	}

	return uint64(ifd.InOctets)
}

func (ifd *IfData) GetInOctetsMaxValue() uint64 {
	if ifd.HCInOctets != 0 {
		return math.MaxUint64
	}

	return uint64(math.MaxUint32)
}

func (ifd *IfData) GetOutOctets() uint64 {
	if ifd.HCOutOctets != 0 {
		return ifd.HCOutOctets
	}

	return uint64(ifd.OutOctets)
}

func (ifd *IfData) GetOutOctetsMaxValue() uint64 {
	if ifd.HCOutOctets != 0 {
		return math.MaxUint64
	}

	return uint64(math.MaxUint32)
}

func (ifd *IfData) GetAllInPackets() uint64 {
	if ifd.HCInUcastPkts != 0 || ifd.HCInBroadcastPkts != 0 || ifd.HCInMulticastPkts != 0 {
		return ifd.HCInUcastPkts + ifd.HCInBroadcastPkts + ifd.HCInMulticastPkts
	}

	return uint64(ifd.InUcastPkts)
}

func (ifd *IfData) GetAllOutPackets() uint64 {
	if ifd.HCOutUcastPkts != 0 || ifd.HCOutBroadcastPkts != 0 || ifd.HCOutMulticastPkts != 0 {
		return ifd.HCOutUcastPkts + ifd.HCOutBroadcastPkts + ifd.HCOutMulticastPkts
	}

	return uint64(ifd.InUcastPkts)
}

func (ifd *IfData) GetInErrors() uint64 {
	return uint64(ifd.InErrors)
}

func (ifd *IfData) GetOutErrors() uint64 {
	return uint64(ifd.OutErrors)
}

func (ifd *IfData) GetInDiscards() uint64 {
	return uint64(ifd.InDiscards)
}

func (ifd *IfData) GetOutDiscards() uint64 {
	return uint64(ifd.OutDiscards)
}
