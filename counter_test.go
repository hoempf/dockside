package main

import "testing"

func TestCounter_IncDecValue(t *testing.T) {
	counter := NewCounter()
	type args struct {
		key string
	}
	tests := []struct {
		name string
		c    *Counter
		args args
		want int
	}{
		{
			name: "simple increment",
			c:    counter,
			args: args{
				key: "key",
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Inc(tt.args.key); got != tt.want {
				t.Errorf("Counter.Inc() = %v, want %v", got, tt.want)
			}
			if got := tt.c.Value(tt.args.key); got != tt.want {
				t.Errorf("Counter.Value() = %v, want %v", got, tt.want)
			}
			if got := tt.c.Dec(tt.args.key); got != tt.want-1 {
				t.Errorf("Counter.Dec() = %v, want %v", got, tt.want)
			}
		})
	}
}
