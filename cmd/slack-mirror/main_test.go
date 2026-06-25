package main

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    command
		wantErr bool
	}{
		{"serve", []string{"serve"}, cmdServe, false},
		{"backfill", []string{"backfill"}, cmdBackfill, false},
		{"no args", []string{}, cmdNone, true},
		{"unknown", []string{"frobnicate"}, cmdNone, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommand(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
