package data

import (
	"math/rand"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	commonslices "github.com/avanha/pmaas-common/slices"
)

type globals struct {
	denverLocation  *time.Location
	localTimeStamp1 time.Time
	localTimeStamp2 time.Time
	data            NetInterfaceData
}

var g = globals{}

const Giga = 1024 * 1024 * 1024

func TestMain(m *testing.M) {
	//largeFloat := big.NewFloat(2.4 * float64(Giga) / 8.0)
	//maxRateHistoryRateValue, _ := largeFloat.Int64()

	location, _ := time.LoadLocation("America/Denver")
	timeStamp, _ := time.Parse(time.RFC3339, "2020-03-12T15:04:23Z")
	g.denverLocation = location
	g.localTimeStamp1 = timeStamp.In(location)
	g.localTimeStamp2 = g.localTimeStamp1.Add(2*time.Hour + 31*time.Minute)
	g.data = NetInterfaceData{
		Index:         1,
		Name:          "testName",
		Status:        "Up",
		PhysAddress:   "12:34:56:78:90:AB",
		IpV4Addresses: []string{"192.168.1.1", "10.0.0.1"},
		IpAddresses: []net.IP{
			net.ParseIP("192.168.1.1"),
			net.ParseIP("10.0.0.1"),
			net.ParseIP("::1")},
		LastIpV4AddressChangeTime: g.localTimeStamp1,
		BytesIn:                   100,
		BytesOut:                  200,
		PacketsIn:                 300,
		PacketsOut:                400,
		ErrorsIn:                  500,
		ErrorsOut:                 600,
		DiscardsIn:                700,
		DiscardsOut:               800,
		LastUpdateTime:            g.localTimeStamp2,
		BytesInRateHistory:        [64]uint64(genRateHistory(true, 0, 64)),
		BytesOutRateHistory:       [64]uint64(genRateHistory(false, 63, 64)),
	}
	os.Exit(m.Run())
}

func genRateHistory(forward bool, start int, count int) []uint64 {
	if count > NetInterfaceDataHistorySize {
		panic("count cannot be greater than NetInterfaceDataHistorySize")
	}

	result := make([]uint64, count)

	if forward {
		for i := 0; i < count; i++ {
			result[i] = uint64((start + i) % NetInterfaceDataHistorySize)
		}
	} else {
		for i := 0; i < count; i++ {
			result[i] = uint64((start + NetInterfaceDataHistorySize - i) % NetInterfaceDataHistorySize)
		}
	}

	return result
}

func genRandomRateHistory(minValue int64, maxValue int64) [NetInterfaceDataHistorySize]uint64 {
	result := [NetInterfaceDataHistorySize]uint64{}

	// Local RNG to avoid shared global state between tests
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for index := range NetInterfaceDataHistorySize {
		result[index] = uint64(minValue + rng.Int63n(maxValue-minValue+1))
	}

	return result
}

func TestNetInterfaceDataToInsertArgs_ConvertsAllValues(t *testing.T) {
	var dataAsAny any = g.data
	args, err := NetInterfaceDataToInsertArgs(&dataAsAny)

	if err != nil {
		t.Error(err)
	}

	expectedArgs := []any{g.data.Index,
		g.data.Name,
		g.data.Status,
		g.data.PhysAddress,
		strings.Join(g.data.IpV4Addresses, ","),
		strings.Join(commonslices.Apply(g.data.IpAddresses, IpToString), ","),
		g.data.BytesIn,
		g.data.BytesOut,
		g.data.PacketsIn,
		g.data.PacketsOut,
		g.data.ErrorsIn,
		g.data.ErrorsOut,
		g.data.DiscardsIn,
		g.data.DiscardsOut,
		g.data.LastUpdateTime}

	if !slices.Equal(args, expectedArgs) {
		t.Errorf("Expected %v, got %v", expectedArgs, args)
	}
}

func IpToString(ipAddress *net.IP) string {
	return ipAddress.String()
}

func TestNetInterfaceDataToInsertArgs_ConvertsEmptyStringsToNil(t *testing.T) {
	g.data.PhysAddress = ""
	g.data.IpV4Addresses = []string{}
	g.data.IpAddresses = []net.IP{}
	g.data.LastUpdateTime = time.Time{}

	var dataAsAny any = g.data
	args, err := NetInterfaceDataToInsertArgs(&dataAsAny)

	if err != nil {
		t.Error(err)
	}

	expectedArgs := []any{g.data.Index,
		g.data.Name,
		g.data.Status,
		nil,
		nil,
		nil,
		g.data.BytesIn,
		g.data.BytesOut,
		g.data.PacketsIn,
		g.data.PacketsOut,
		g.data.ErrorsIn,
		g.data.ErrorsOut,
		g.data.DiscardsIn,
		g.data.DiscardsOut,
		nil}

	if !slices.Equal(args, expectedArgs) {
		t.Errorf("Expected %v, got %v", expectedArgs, args)
	}
}

func TestNetInterfaceDataBytesInRateHistory_returnCorrectResults(t *testing.T) {
	expected := genRateHistory(true, 1, 64)

	actual := g.data.GetBytesInRateHistory(64)

	if !slices.Equal(expected, actual) {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestNetInterfaceDataBytesInRateHistory_midpointInHistoryBuffer_returnCorrectResults(t *testing.T) {
	expected := genRateHistory(true, 16, 64)
	g.data.CurrentHistoryIndex = 15

	actual := g.data.GetBytesInRateHistory(64)

	if !slices.Equal(expected, actual) {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestNetInterfaceDataBytesInRateHistory_midpointInHistoryBuffer_subset_returnCorrectResults(t *testing.T) {
	expected := []uint64{62, 63, 0, 1}
	g.data.CurrentHistoryIndex = 1

	actual := g.data.GetBytesInRateHistory(4)

	if !slices.Equal(expected, actual) {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestNetInterfaceDataGetDailyHistory_ContiguousRead(t *testing.T) {
	var src [NetInterfaceDailyHistorySize]uint64
	for i := 0; i < NetInterfaceDailyHistorySize; i++ {
		src[i] = uint64(i)
	}

	// Read 5 items ending at index 10. Start index should be 6.
	actual := GetDailyHistory(&src, 10, 5)
	expected := []uint64{6, 7, 8, 9, 10}

	if !slices.Equal(actual, expected) {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestNetInterfaceDataGetDailyHistory_WrapAround(t *testing.T) {
	var src [NetInterfaceDailyHistorySize]uint64
	for i := 0; i < NetInterfaceDailyHistorySize; i++ {
		src[i] = uint64(i)
	}

	// Read 5 items ending at index 2. Start index should be 62 (for size 64).
	actual := GetDailyHistory(&src, 2, 5)
	expected := []uint64{62, 63, 0, 1, 2}

	if !slices.Equal(actual, expected) {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestNetInterfaceDataGetDailyHistory_LimitExceedsBufferSie_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()

	var src [NetInterfaceDailyHistorySize]uint64
	for i := 0; i < NetInterfaceDailyHistorySize; i++ {
		src[i] = uint64(i)
	}

	// Requesting more than the buffer size should clamp to buffer size
	GetDailyHistory(&src, int(NetInterfaceDailyHistorySize-1), 100)
}

func TestNetInterfaceDataGetCurrentMonthTotalBytes_Normal(t *testing.T) {
	d := &NetInterfaceData{}

	// Let's pretend it's the 3rd of the month, so we need to read 3 days
	d.LastUpdateTime = time.Date(2023, 10, 3, 12, 0, 0, 0, time.UTC)
	d.CurrentDayIndex = 2 // Index 2 is the 3rd day

	d.DailyBytesIn[0] = 10
	d.DailyBytesIn[1] = 20
	d.DailyBytesIn[2] = 30
	d.DailyBytesOut[0] = 100
	d.DailyBytesOut[1] = 200
	d.DailyBytesOut[2] = 300

	// Previous month data shouldn't be included
	d.DailyBytesIn[63] = 999
	d.DailyBytesOut[63] = 999

	in, out := d.GetCurrentMonthTotalBytes()

	if in != 60 {
		t.Errorf("Expected 60 total in, got %d", in)
	}
	if out != 600 {
		t.Errorf("Expected 600 total out, got %d", out)
	}
}

func TestNetInterfaceDataGetCurrentMonthTotalBytes_Rollover(t *testing.T) {
	d := &NetInterfaceData{}

	// Let's pretend it's the 3rd of the month
	d.LastUpdateTime = time.Date(2023, 10, 3, 12, 0, 0, 0, time.UTC)
	d.CurrentDayIndex = 1 // Index 1 is the 3rd day. Wrapped around.

	// Day 3 (index 1)
	d.DailyBytesIn[1] = 30
	d.DailyBytesOut[1] = 300
	// Day 2 (index 0)
	d.DailyBytesIn[0] = 20
	d.DailyBytesOut[0] = 200
	// Day 1 (index 63 - rolled over)
	d.DailyBytesIn[63] = 10
	d.DailyBytesOut[63] = 100

	// Previous month data shouldn't be included
	d.DailyBytesIn[62] = 999
	d.DailyBytesOut[62] = 999

	in, out := d.GetCurrentMonthTotalBytes()

	if in != 60 {
		t.Errorf("Expected 60 total in, got %d", in)
	}
	if out != 600 {
		t.Errorf("Expected 600 total out, got %d", out)
	}
}

func TestNetInterfaceDataGetCurrentMonthTotalBytes_ZeroTime(t *testing.T) {
	d := &NetInterfaceData{}
	in, out := d.GetCurrentMonthTotalBytes()
	if in != 0 || out != 0 {
		t.Errorf("Expected 0, 0 for zero time, got %d, %d", in, out)
	}
}
