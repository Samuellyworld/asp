package cmd

import (
	"testing"
	"time"
)

type fakeScannerStatus struct {
	running bool
	last    time.Time
}

func (f fakeScannerStatus) IsRunning() bool        { return f.running }
func (f fakeScannerStatus) LastCycleAt() time.Time { return f.last }

func TestInfraWatchdogScannerFreshness(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		status  fakeScannerStatus
		setAt   time.Time
		wantErr bool
	}{
		{
			name:    "fresh scanner",
			status:  fakeScannerStatus{running: true, last: now.Add(-2 * time.Minute)},
			setAt:   now.Add(-10 * time.Minute),
			wantErr: false,
		},
		{
			name:    "stale scanner",
			status:  fakeScannerStatus{running: true, last: now.Add(-30 * time.Minute)},
			setAt:   now.Add(-30 * time.Minute),
			wantErr: true,
		},
		{
			name:    "not running",
			status:  fakeScannerStatus{running: false, last: now},
			setAt:   now,
			wantErr: true,
		},
		{
			name:    "initial cycle still pending",
			status:  fakeScannerStatus{running: true},
			setAt:   now,
			wantErr: false,
		},
		{
			name:    "initial cycle stale",
			status:  fakeScannerStatus{running: true},
			setAt:   now.Add(-30 * time.Minute),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewInfraWatchdog(nil, nil, time.Minute)
			w.scanner = tt.status
			w.scannerSetAt = tt.setAt
			w.scannerStale = 15 * time.Minute

			err := w.checkScannerFreshness()
			if tt.wantErr && err == nil {
				t.Fatal("expected scanner freshness error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected scanner freshness error: %v", err)
			}
		})
	}
}
