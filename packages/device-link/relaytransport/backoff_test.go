package relaytransport

import (
	"math"
	"testing"
	"time"
)

func TestExponentialBackoffUsesBoundedFullJitter(t *testing.T) {
	inputs := []int64{0, 25, 100, 399, 999}
	index := 0
	backoff := newExponentialBackoff(BackoffConfig{
		Initial:    100 * time.Millisecond,
		Max:        time.Second,
		Multiplier: 2,
		RandInt63n: func(limit int64) int64 {
			value := inputs[index]
			index++
			if value >= limit {
				t.Fatalf("random input %d is outside limit %d", value, limit)
			}
			return value
		},
	})

	want := []time.Duration{0, 25, 100, 399, 999}
	for i, expected := range want {
		if got := backoff.Next(); got != expected {
			t.Fatalf("Next() call %d = %s, want %s", i+1, got, expected)
		}
	}
}

func TestExponentialBackoffResetRestartsGeneration(t *testing.T) {
	var limits []int64
	backoff := newExponentialBackoff(BackoffConfig{
		Initial:    100 * time.Millisecond,
		Max:        time.Second,
		Multiplier: 2,
		RandInt63n: func(limit int64) int64 {
			limits = append(limits, limit)
			return 0
		},
	})

	backoff.Next()
	backoff.Next()
	backoff.Reset()
	backoff.Next()

	want := []int64{
		int64(100*time.Millisecond) + 1,
		int64(200*time.Millisecond) + 1,
		int64(100*time.Millisecond) + 1,
	}
	if len(limits) != len(want) {
		t.Fatalf("random limits = %v, want %v", limits, want)
	}
	for i := range want {
		if limits[i] != want[i] {
			t.Fatalf("random limit %d = %d, want %d", i+1, limits[i], want[i])
		}
	}
}

func TestExponentialBackoffSaturatesWithoutOverflow(t *testing.T) {
	var limits []int64
	backoff := newExponentialBackoff(BackoffConfig{
		Initial:    time.Duration(math.MaxInt64 - 1),
		Max:        time.Duration(math.MaxInt64),
		Multiplier: 2,
		RandInt63n: func(limit int64) int64 {
			limits = append(limits, limit)
			return limit - 1
		},
	})

	first := backoff.Next()
	second := backoff.Next()
	if first != time.Duration(math.MaxInt64-1) {
		t.Fatalf("first delay = %s, want MaxInt64-1", first)
	}
	if second != time.Duration(math.MaxInt64-1) {
		t.Fatalf("second delay = %s, want MaxInt64-1", second)
	}
	if len(limits) != 2 || limits[0] != math.MaxInt64 || limits[1] != math.MaxInt64 {
		t.Fatalf("random limits = %v, want two MaxInt64 limits", limits)
	}
}
