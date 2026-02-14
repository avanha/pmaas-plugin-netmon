package data

import (
	"math/rand"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
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
		Index:                     1,
		Name:                      "testName",
		PhysAddress:               "12:34:56:78:90:AB",
		IpV4Addresses:             []string{"192.168.1.1", "10.0.0.1"},
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
		g.data.PhysAddress,
		strings.Join(g.data.IpV4Addresses, ","),
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

func TestNetInterfaceDataToInsertArgs_ConvertsEmptyStringsToNil(t *testing.T) {
	g.data.PhysAddress = ""
	g.data.IpV4Addresses = []string{}
	g.data.LastUpdateTime = time.Time{}

	var dataAsAny any = g.data
	args, err := NetInterfaceDataToInsertArgs(&dataAsAny)

	if err != nil {
		t.Error(err)
	}

	expectedArgs := []any{g.data.Index,
		g.data.Name,
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
