package main

import (
	"context"
	"fmt"
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
			t.Logf("found %d containers", tt.d.list.Len())
		})
	}
}

func TestContainer_Equals(t *testing.T) {
	type args struct {
		other *Container
	}
	tests := []struct {
		name string
		c    *Container
		args args
		want bool
	}{
		{
			name: "equality",
			c:    &Container{ID: "myid", Name: "test"},
			args: args{
				other: &Container{ID: "myid", Name: "test2"},
			},
			want: true,
		},
		{
			name: "inequality",
			c:    &Container{ID: "myid", Name: "test"},
			args: args{
				other: &Container{ID: "myid2", Name: "test"},
			},
			want: false,
		},
		{
			name: "zerovalue",
			c:    &Container{},
			args: args{
				other: &Container{},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Equals(tt.args.other); got != tt.want {
				t.Errorf("Container.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerList_Upsert(t *testing.T) {
	type wantFunc func(got *ContainerList) error

	list := NewContainerList()
	type args struct {
		c *Container
	}
	tests := []struct {
		name string
		l    *ContainerList
		args args
		want wantFunc
	}{
		{
			name: "insert",
			l:    list,
			args: args{
				c: &Container{ID: "test", Name: "huii"},
			},
			want: func(got *ContainerList) error {
				if got.Len() != 1 {
					return fmt.Errorf("len(list) = %d, want %d", got.Len(), 1)
				}
				if got.list[0].ID != "test" {
					return fmt.Errorf("Upsert() did not work, got = %+v", got.list)
				}
				return nil
			},
		},
		{
			name: "update",
			l:    list,
			args: args{
				c: &Container{ID: "test", Name: "huii2"},
			},
			want: func(got *ContainerList) error {
				if got.Len() != 1 {
					return fmt.Errorf("len(list) = %d, want %d", got.Len(), 1)
				}
				if got.list[0].Name != "huii2" {
					return fmt.Errorf("Upserted.Name = %s, want %s", got.list[0].Name, "huii2")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.l.Upsert(tt.args.c)
			if err := tt.want(tt.l); err != nil {
				t.Error(err)
			}
		})
	}
}
