package data

import (
	"net"
	"reflect"
	"strings"
	"time"

	commonslices "github.com/avanha/pmaas-common/slices"
)

const NetInterfaceDataHistorySize = 64

type NetInterfaceData struct {
	Index                     uint32   `track:"onchange"`
	Name                      string   `track:"onchange,maxLength=255"`
	Status                    string   `track:"onchange,maxLength=30"`
	PhysAddress               string   `track:"onchange,name=PhysicalAddress,maxLength=100"`
	IpV4Addresses             []string `track:"onchange,dataType=varchar,maxLength=150"`
	LastIpV4AddressChangeTime time.Time
	IpAddresses               []net.IP `track:"onchange,dataType=varchar,maxLength=255"`
	LastIpAddressesChangeTime time.Time
	BytesIn                   uint64    `track:"always"`
	BytesOut                  uint64    `track:"always"`
	PacketsIn                 uint64    `track:"always"`
	PacketsOut                uint64    `track:"always"`
	ErrorsIn                  uint64    `track:"always"`
	ErrorsOut                 uint64    `track:"always"`
	DiscardsIn                uint64    `track:"always"`
	DiscardsOut               uint64    `track:"always"`
	LastUpdateTime            time.Time `track:"always"`
	CurrentHistoryIndex       uint
	BytesInRateHistory        [NetInterfaceDataHistorySize]uint64
	BytesOutRateHistory       [NetInterfaceDataHistorySize]uint64
}

func (d NetInterfaceData) GetBytesInRateHistory(limit int) []uint64 {
	return GetHistory(&d.BytesInRateHistory, d.CurrentHistoryIndex, limit)
}

func (d NetInterfaceData) GetBytesOutRateHistory(limit int) []uint64 {
	return GetHistory(&d.BytesOutRateHistory, d.CurrentHistoryIndex, limit)
}

// GetHistory retrieves a slice of the recent history data up to the specified limit.  The history is returned
// in chronological order, with the oldest entry in position zero of the result slice.
func GetHistory(src *[NetInterfaceDataHistorySize]uint64, currentIndex uint, limit int) []uint64 {
	// Clamp limit to buffer size
	if limit > NetInterfaceDataHistorySize {
		limit = NetInterfaceDataHistorySize
	}

	// Use int for slice indexing and calculations
	curr := int(currentIndex)

	result := make([]uint64, limit)

	// Calculate start index (chronological start)
	// Logic: End is at currentIndex. Start is 'limit - 1' steps back.
	// We add 'size' before subtracting to handle the wrap-around case cleanly.
	startIdx := (curr + NetInterfaceDataHistorySize + 1 - limit) & (NetInterfaceDataHistorySize - 1)

	// Determine if the range wraps around the end of the buffer
	firstChunkLen := NetInterfaceDataHistorySize - startIdx

	if limit <= firstChunkLen {
		// Contiguous read (no wrap-around)
		copy(result, src[startIdx:startIdx+limit])
	} else {
		// Wrap-around read
		// 1. Copy from startIdx to the end of the buffer
		copy(result, src[startIdx:])
		// 2. Copy the remaining items from the beginning of the buffer
		copy(result[firstChunkLen:], src[:limit-firstChunkLen])
	}

	return result
}

var NetInterfaceDataType = reflect.TypeOf((*NetInterfaceData)(nil)).Elem()

func NetInterfaceDataToInsertArgs(genericDataPointer *any) ([]any, error) {
	data := (*genericDataPointer).(NetInterfaceData)
	args := []any{
		data.Index,
		data.Name,
		data.Status,
		stringEmptyToNil(data.PhysAddress),
		stringEmptyToNil(strings.Join(data.IpV4Addresses, ",")),
		stringEmptyToNil(
			strings.Join(
				commonslices.Apply(
					data.IpAddresses,
					func(ip *net.IP) string { return ip.String() }),
				","),
		),
		data.BytesIn,
		data.BytesOut,
		data.PacketsIn,
		data.PacketsOut,
		data.ErrorsIn,
		data.ErrorsOut,
		data.DiscardsIn,
		data.DiscardsOut,
		timeEmptyToNil(data.LastUpdateTime),
	}
	return args, nil
}

func stringEmptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func timeEmptyToNil(time time.Time) any {
	if time.IsZero() {
		return nil
	}
	return time
}
