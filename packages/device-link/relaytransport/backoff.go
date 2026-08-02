package relaytransport

import (
	"math"
	"math/rand"
	"time"
)

type exponentialBackoff struct {
	cfg     BackoffConfig
	current time.Duration
}

func newExponentialBackoff(cfg BackoffConfig) *exponentialBackoff {
	if cfg.Initial <= 0 {
		cfg.Initial = 100 * time.Millisecond
	}
	if cfg.Max <= 0 {
		cfg.Max = 5 * time.Second
	}
	if cfg.Multiplier <= 1 || math.IsNaN(cfg.Multiplier) || math.IsInf(cfg.Multiplier, 0) {
		cfg.Multiplier = 2
	}
	if cfg.Initial > cfg.Max {
		cfg.Initial = cfg.Max
	}
	if cfg.RandInt63n == nil {
		cfg.RandInt63n = rand.Int63n
	}
	return &exponentialBackoff{cfg: cfg}
}

func (b *exponentialBackoff) Reset() { b.current = 0 }

func (b *exponentialBackoff) Next() time.Duration {
	if b.current == 0 {
		b.current = b.cfg.Initial
	} else if float64(b.current) >= float64(b.cfg.Max)/b.cfg.Multiplier {
		b.current = b.cfg.Max
	} else {
		next := time.Duration(float64(b.current) * b.cfg.Multiplier)
		if next <= b.current || next > b.cfg.Max {
			b.current = b.cfg.Max
		} else {
			b.current = next
		}
	}
	if b.current <= 0 {
		return 0
	}
	if b.current == time.Duration(math.MaxInt64) {
		return time.Duration(b.cfg.RandInt63n(math.MaxInt64))
	}
	return time.Duration(b.cfg.RandInt63n(int64(b.current) + 1))
}
