package main

import (
	"context"
	"testing"
)

func newDockwatch(ctx context.Context, t *testing.T) *Dockwatch {
	d, err := NewDockwatch(ctx)
	if err != nil {
		t.Error(err)
	}
	return d
}

func TestDockwatch_WatchContainer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	tests := []struct {
		name    string
		d       *Dockwatch
		wantErr bool
	}{
		{
			name:    "init list",
			d:       newDockwatch(ctx, t),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.d.WatchContainer(); (err != nil) != tt.wantErr {
				t.Errorf("Dockwatch.WatchContainer() error = %v, wantErr %v", err, tt.wantErr)
			}
			t.Logf("found %d containers", len(tt.d.list))
		})
	}
}
