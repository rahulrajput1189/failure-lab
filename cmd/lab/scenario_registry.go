package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ScenarioManifest struct {
	SchemaVersion int      `yaml:"schemaVersion"`
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	Tags          []string `yaml:"tags"`
}

type RegisteredScenario struct {
	Manifest ScenarioManifest
	Handler  Scenario
}

var scenarioIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func loadScenarioRegistry() ([]RegisteredScenario, error) {
	root := projectRoot()
	scenarioRoot := filepath.Join(root, "scenarios")

	entries, err := os.ReadDir(scenarioRoot)
	if err != nil {
		return nil, fmt.Errorf("read scenarios directory %q: %w", scenarioRoot, err)
	}

	ids := make(map[string]struct{})
	registered := make([]RegisteredScenario, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		manifestPath := filepath.Join(scenarioRoot, dirName, "scenario.yaml")

		manifest, err := readScenarioManifest(manifestPath)
		if err != nil {
			return nil, err
		}

		if err := validateScenarioManifest(&manifest, dirName); err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}

		if _, exists := ids[manifest.ID]; exists {
			return nil, fmt.Errorf("duplicate scenario ID %q", manifest.ID)
		}
		ids[manifest.ID] = struct{}{}

		handler, exists := scenarios[manifest.ID]
		if !exists {
			return nil, fmt.Errorf(
				"manifest %q has no registered Go scenario handler",
				manifest.ID,
			)
		}

		registered = append(registered, RegisteredScenario{
			Manifest: manifest,
			Handler:  handler,
		})
	}

	for id := range scenarios {
		if _, exists := ids[id]; !exists {
			return nil, fmt.Errorf(
				"registered Go scenario %q has no scenario.yaml manifest",
				id,
			)
		}
	}

	sort.Slice(registered, func(i, j int) bool {
		return registered[i].Manifest.ID < registered[j].Manifest.ID
	})

	return registered, nil
}

func readScenarioManifest(path string) (ScenarioManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return ScenarioManifest{}, fmt.Errorf(
			"open scenario manifest %q: %w",
			path,
			err,
		)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)

	var manifest ScenarioManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ScenarioManifest{}, fmt.Errorf(
			"parse scenario manifest %q: %w",
			path,
			err,
		)
	}

	var extra yaml.Node
	err = decoder.Decode(&extra)
	if err != io.EOF {
		if err == nil {
			return ScenarioManifest{}, fmt.Errorf(
				"parse scenario manifest %q: multiple YAML documents are not allowed",
				path,
			)
		}
		return ScenarioManifest{}, fmt.Errorf(
			"parse trailing content in scenario manifest %q: %w",
			path,
			err,
		)
	}

	return manifest, nil
}

func validateScenarioManifest(manifest *ScenarioManifest, directoryName string) error {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Description = strings.TrimSpace(manifest.Description)

	if manifest.SchemaVersion != 1 {
		return fmt.Errorf(
			"unsupported schemaVersion %d; supported version is 1",
			manifest.SchemaVersion,
		)
	}
	if manifest.ID == "" {
		return fmt.Errorf("id must not be empty")
	}
	if !scenarioIDPattern.MatchString(manifest.ID) {
		return fmt.Errorf(
			"id %q must contain lowercase letters, digits, and single hyphens only",
			manifest.ID,
		)
	}
	if manifest.ID != directoryName {
		return fmt.Errorf(
			"id %q must match scenario directory name %q",
			manifest.ID,
			directoryName,
		)
	}
	if manifest.Name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if manifest.Description == "" {
		return fmt.Errorf("description must not be empty")
	}
	if len(manifest.Tags) == 0 {
		return fmt.Errorf("tags must contain at least one non-empty value")
	}

	seenTags := make(map[string]struct{}, len(manifest.Tags))
	for i, tag := range manifest.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return fmt.Errorf("tag at index %d must not be empty", i)
		}
		if _, exists := seenTags[tag]; exists {
			return fmt.Errorf("duplicate tag %q", tag)
		}
		seenTags[tag] = struct{}{}
		manifest.Tags[i] = tag
	}

	return nil
}

func getRegisteredScenario(id string) (RegisteredScenario, error) {
	registered, err := loadScenarioRegistry()
	if err != nil {
		return RegisteredScenario{}, err
	}

	for _, scenario := range registered {
		if scenario.Manifest.ID == id {
			return scenario, nil
		}
	}

	available := make([]string, 0, len(registered))
	for _, scenario := range registered {
		available = append(available, scenario.Manifest.ID)
	}

	if len(available) == 0 {
		return RegisteredScenario{}, fmt.Errorf(
			"unknown scenario %q; no scenarios are registered",
			id,
		)
	}

	return RegisteredScenario{}, fmt.Errorf(
		"unknown scenario %q; available scenarios: %s",
		id,
		strings.Join(available, ", "),
	)
}

func listRegisteredScenarios() error {
	registered, err := loadScenarioRegistry()
	if err != nil {
		return err
	}

	fmt.Println("Available scenarios:")
	fmt.Println()

	for _, scenario := range registered {
		fmt.Printf("%s — %s\n", scenario.Manifest.ID, scenario.Manifest.Name)
		fmt.Printf("  %s\n", scenario.Manifest.Description)
		fmt.Printf("  Tags: %s\n", strings.Join(scenario.Manifest.Tags, ", "))
		fmt.Println()
	}

	return nil
}

// Keep YAML parsing strict and reject empty or whitespace-only manifest files.
func isEmptyScenarioManifest(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}
