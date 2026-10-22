package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseScenarioRunArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantID   string
		wantSeed int64
		wantErr  bool
	}{
		{name: "default seed", args: []string{"inventory-reservation-leak"}, wantID: "inventory-reservation-leak", wantSeed: 1},
		{name: "explicit seed", args: []string{"inventory-reservation-leak", "--seed", "42"}, wantID: "inventory-reservation-leak", wantSeed: 42},
		{name: "missing scenario", args: nil, wantErr: true},
		{name: "missing seed", args: []string{"scenario", "--seed"}, wantErr: true},
		{name: "negative seed", args: []string{"scenario", "--seed", "-1"}, wantErr: true},
		{name: "noninteger seed", args: []string{"scenario", "--seed", "abc"}, wantErr: true},
		{name: "unknown option", args: []string{"scenario", "--other"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, seed, err := parseScenarioRunArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (id != tt.wantID || seed != tt.wantSeed) {
				t.Fatalf("got (%q, %d), want (%q, %d)", id, seed, tt.wantID, tt.wantSeed)
			}
		})
	}
}

func TestWriteScenarioResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	want := ScenarioResult{SchemaVersion: 1, ScenarioID: "example", ScenarioName: "Example", Seed: 7, Status: "passed", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), FinishedAt: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC), DurationMS: 1000, ArtifactDir: "/tmp/run"}
	if err := writeScenarioResult(path, want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got ScenarioResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ScenarioID != want.ScenarioID || got.Seed != want.Seed || got.Status != want.Status || got.DurationMS != want.DurationMS {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatal("result JSON must end with a newline")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary result file should not remain; stat error=%v", err)
	}
}

func TestInvokeScenarioHandlerReturnsHandlerError(t *testing.T) {
	want := "expected handler error"
	err := invokeScenarioHandler(func() error {
		return fmt.Errorf("%s", want)
	})
	if err == nil || err.Error() != want {
		t.Fatalf("got error %v, want %q", err, want)
	}
}

func TestInvokeScenarioHandlerRecoversPanic(t *testing.T) {
	err := invokeScenarioHandler(func() error {
		panic("test panic")
	})
	if err == nil {
		t.Fatal("expected panic to be converted to an error")
	}
	if !strings.Contains(err.Error(), "scenario handler panicked: test panic") {
		t.Fatalf("unexpected error: %v", err)
	}
}
