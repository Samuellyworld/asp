package leverage

import (
	"sync"
	"time"
)

// records funding fee payments per position and provides cumulative totals.
// binance charges funding fees every 8 hours (00:00, 08:00, 16:00 UTC).
type FundingPayment struct {
	PositionID string
	Rate       float64
	Amount     float64
	Timestamp  time.Time
}

// tracks funding fee payments across positions
type FundingTracker struct {
	mu       sync.Mutex
	payments map[string][]FundingPayment // position id -> payments
}

// creates a new funding tracker with an initialized payments map
func NewFundingTracker() *FundingTracker {
	return &FundingTracker{
		payments: make(map[string][]FundingPayment),
	}
}

// records a funding fee payment for a position.
// the amount is calculated as rate * notional.
func (t *FundingTracker) RecordPayment(positionID string, rate float64, notional float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	amount := rate * notional
	payment := FundingPayment{
		PositionID: positionID,
		Rate:       rate,
		Amount:     amount,
		Timestamp:  time.Now().UTC(),
	}
	t.payments[positionID] = append(t.payments[positionID], payment)
}

// returns the cumulative funding fees for a position
func (t *FundingTracker) CumulativeFees(positionID string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	var total float64
	for _, p := range t.payments[positionID] {
		total += p.Amount
	}
	return total
}

// returns all payments for a position.
// returns nil if no payments exist for the given position.
func (t *FundingTracker) Payments(positionID string) []FundingPayment {
	t.mu.Lock()
	defer t.mu.Unlock()

	src := t.payments[positionID]
	if src == nil {
		return nil
	}
	out := make([]FundingPayment, len(src))
	copy(out, src)
	return out
}

// fundingHours are the UTC hours when binance charges funding fees
var fundingHours = []int{0, 8, 16}

// checks if a funding fee is due based on the last payment time.
// funding happens at 00:00, 08:00, 16:00 UTC.
// returns true if there are no previous payments or if a funding
// window has passed since the last payment.
func (t *FundingTracker) IsFundingDue(positionID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	payments := t.payments[positionID]
	if len(payments) == 0 {
		return true
	}

	lastPayment := payments[len(payments)-1].Timestamp
	now := time.Now().UTC()

	// find the most recent funding time that is at or before now
	latestFunding := mostRecentFundingTime(now)

	// funding is due if the latest funding time is after the last payment
	return latestFunding.After(lastPayment)
}

// mostRecentFundingTime returns the most recent funding time at or before the given time.
// funding times are 00:00, 08:00, 16:00 UTC daily.
func mostRecentFundingTime(t time.Time) time.Time {
	t = t.UTC()
	hour := t.Hour()
	y, m, d := t.Date()

	var fundingHour int
	switch {
	case hour >= 16:
		fundingHour = 16
	case hour >= 8:
		fundingHour = 8
	default:
		fundingHour = 0
	}

	return time.Date(y, m, d, fundingHour, 0, 0, 0, time.UTC)
}

// removes all tracking data for a position
func (t *FundingTracker) Cleanup(positionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.payments, positionID)
}

// returns total fees across all positions
func (t *FundingTracker) TotalFees() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	var total float64
	for _, payments := range t.payments {
		for _, p := range payments {
			total += p.Amount
		}
	}
	return total
}
