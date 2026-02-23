package http

import (
	"testing"
	"time"
)

func TestFormatShortDuration_ExactSeconds_WholeSeconds(t *testing.T) {
	d := 11 * time.Second
	result := FormatShortDuration(d)
	expected := "11s"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MoreThanOneSecondPartial_ReturnsFractionalSeconds(t *testing.T) {
	d := 5*time.Second + 149*time.Millisecond
	result := FormatShortDuration(d)
	expected := "5.15s"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MoreThanOneSecondPartial2_ReturnsFractionalSecondsRounded(t *testing.T) {
	d := 5*time.Second + 159*time.Millisecond
	result := FormatShortDuration(d)
	var expected = "5.16s"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MultipleWholeMillisecond_ReturnsWholeMilliseconds(t *testing.T) {
	d := 15 * time.Millisecond
	result := FormatShortDuration(d)
	var expected = "15ms"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MultipleMillisecondsAndMicros_ReturnsFractionalMillis(t *testing.T) {
	d := 9*time.Millisecond + 125*time.Microsecond
	result := FormatShortDuration(d)
	var expected = "9.12ms"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MoreThan100MillisecondsAndMicros_ReturnsWholeMillis(t *testing.T) {
	d := 150*time.Millisecond + 123*time.Microsecond
	result := FormatShortDuration(d)
	var expected = "150ms"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MoreThan1000WholeMicros_ReturnsWholeMicros(t *testing.T) {
	d := 1236 * time.Microsecond
	result := FormatShortDuration(d)
	var expected = "1.24ms"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MultipleWholeMicros_ReturnsWholeMicros(t *testing.T) {
	d := 123 * time.Microsecond
	result := FormatShortDuration(d)
	var expected = "123μs"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatShortDuration_MultipleMicrosAndNanos_ReturnsWholeMicros(t *testing.T) {
	d := 123*time.Microsecond + 539*time.Nanosecond
	result := FormatShortDuration(d)
	var expected = "123μs"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}
