package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const scenarioResultSchemaVersion = 1

// activeScenarioSeed is exposed to child services so future randomized scenarios
// can use the same seed. Current Phase 4 scenarios use fixed database fixtures.
var activeScenarioSeed int64 = 1

type ScenarioResult struct {
	SchemaVersion int       `json:"schemaVersion"`
	ScenarioID    string    `json:"scenarioId"`
	ScenarioName  string    `json:"scenarioName"`
	Seed          int64     `json:"seed"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	DurationMS    int64     `json:"durationMs"`
	ArtifactDir   string    `json:"artifactDir"`
	Error         string    `json:"error,omitempty"`
}

func parseScenarioRunArgs(args []string) (string, int64, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", 0, errors.New("usage: lab run <scenario> [--seed <integer>]")
	}
	id := args[0]
	seed := int64(1)
	for i := 1; i < len(args); i++ {
		if args[i] != "--seed" {
			return "", 0, fmt.Errorf("unknown run option %q; usage: lab run <scenario> [--seed <integer>]", args[i])
		}
		if i+1 >= len(args) {
			return "", 0, errors.New("--seed requires an integer value")
		}
		parsed, err := strconv.ParseInt(args[i+1], 10, 64)
		if err != nil || parsed < 0 {
			return "", 0, fmt.Errorf("invalid seed %q: seed must be a non-negative integer", args[i+1])
		}
		seed = parsed
		i++
	}
	return id, seed, nil
}

func invokeScenarioHandler(handler func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("scenario handler panicked: %v", recovered)
		}
	}()
	return handler()
}

func runScenarioWithOptions(args []string) error {
	id, seed, err := parseScenarioRunArgs(args)
	if err != nil {
		return err
	}
	registered, err := getRegisteredScenario(id)
	if err != nil {
		return err
	}
	if registered.Handler.Reproduce == nil {
		return fmt.Errorf("scenario %q has no reproduction handler", id)
	}
	if registered.Handler.Verify == nil {
		return fmt.Errorf("scenario %q has no verification handler", id)
	}

	root := projectRoot()
	started := time.Now().UTC()
	artifactName := fmt.Sprintf("%s-%s-seed-%d", started.Format("20060102T150405.000000000Z"), id, seed)
	artifactDir := filepath.Join(root, ".local", "runs", artifactName)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return fmt.Errorf("create run artifact directory: %w", err)
	}

	result := ScenarioResult{
		SchemaVersion: scenarioResultSchemaVersion,
		ScenarioID:    id,
		ScenarioName:  registered.Manifest.Name,
		Seed:          seed,
		Status:        "failed",
		StartedAt:     started,
		ArtifactDir:   artifactDir,
	}

	logFile, err := os.OpenFile(filepath.Join(artifactDir, "output.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create scenario output log: %w", err)
	}

	fmt.Printf("Running scenario: %s\nSeed: %d\nArtifacts: %s\n\n", registered.Manifest.Name, seed, artifactDir)
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		logFile.Close()
		return fmt.Errorf("capture scenario output: %w", err)
	}
	os.Stdout = writer
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(io.MultiWriter(originalStdout, logFile), reader)
		copyDone <- copyErr
	}()

	activeScenarioSeed = seed
	fmt.Println("→ reproducing failure")
	runErr := invokeScenarioHandler(registered.Handler.Reproduce)
	if runErr == nil {
		fmt.Println("\n→ verifying resulting state")
		runErr = invokeScenarioHandler(registered.Handler.Verify)
	}
	if runErr == nil {
		fmt.Println("\n✓ scenario reproduced and verified")
		result.Status = "passed"
	} else {
		fmt.Printf("\n✗ scenario failed: %v\n", runErr)
		result.Error = runErr.Error()
	}

	os.Stdout = originalStdout
	_ = writer.Close()
	copyErr := <-copyDone
	_ = reader.Close()
	if err := logFile.Sync(); err != nil && runErr == nil {
		runErr = fmt.Errorf("sync scenario output log: %w", err)
		result.Status = "failed"
		result.Error = runErr.Error()
	}
	if err := logFile.Close(); err != nil && runErr == nil {
		runErr = fmt.Errorf("close scenario output log: %w", err)
		result.Status = "failed"
		result.Error = runErr.Error()
	}
	if copyErr != nil && runErr == nil {
		runErr = fmt.Errorf("capture scenario output: %w", copyErr)
		result.Status = "failed"
		result.Error = runErr.Error()
	}

	result.FinishedAt = time.Now().UTC()
	result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	if err := writeScenarioResult(filepath.Join(artifactDir, "result.json"), result); err != nil {
		if runErr != nil {
			return fmt.Errorf("scenario failed: %v; additionally could not write result: %w", runErr, err)
		}
		return fmt.Errorf("write scenario result: %w", err)
	}
	fmt.Printf("\nRun status: %s\nResult: %s\nOutput: %s\n", result.Status, filepath.Join(artifactDir, "result.json"), filepath.Join(artifactDir, "output.log"))
	return runErr
}

func writeScenarioResult(path string, result ScenarioResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result JSON: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
