package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "up":
		run("docker", "compose", "-f", composeFile(), "up", "-d")

	case "down":
		run("docker", "compose", "-f", composeFile(), "down")

	case "status":
		run("docker", "compose", "-f", composeFile(), "ps")

	case "logs":
		run("docker", "compose", "-f", composeFile(), "logs", "-f", "--tail=100")

	case "reset":
		reset()

	case "scenarios":
		listScenarios()

	case "verify":
		if len(args) != 1 {
			fmt.Println("usage: lab verify <scenario>")
			os.Exit(1)
		}

		scenario := getScenario(args[0])

		if err := scenario.Verify(); err != nil {
			fmt.Printf("\n✗ verification failed: %v\n", err)
			os.Exit(1)
		}

	case "reproduce":
		if len(args) != 1 {
			fmt.Println("usage: lab reproduce <scenario>")
			os.Exit(1)
		}

		scenario := getScenario(args[0])

		if scenario.Reproduce == nil {
			fmt.Printf(
				"scenario %q does not support reproduction\n",
				scenario.Name,
			)
			os.Exit(1)
		}

		if err := scenario.Reproduce(); err != nil {
			fmt.Printf("\n✗ reproduction failed: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("unknown command: %s\n\n", command)
		usage()
		os.Exit(1)
	}
}

func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("could not determine current directory: %v\n", err)
		os.Exit(1)
	}

	for {
		marker := filepath.Join(dir, ".failurelab")

		if _, err := os.Stat(marker); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)

		if parent == dir {
			break
		}

		dir = parent
	}

	fmt.Println("could not find FailureLab project")
	fmt.Println("Run lab from inside a FailureLab project.")
	os.Exit(1)

	return ""
}

func composeFile() string {
	return filepath.Join(projectRoot(), "compose.yaml")
}

func runShell(command string) {
	fmt.Printf("→ %s\n\n", command)

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		fmt.Printf("\n✗ command failed: %v\n", err)
		os.Exit(1)
	}
}

func reset() {
	root := projectRoot()
	compose := composeFile()

	fmt.Println("→ resetting FailureLab")

	run(
		"docker",
		"compose",
		"-f",
		compose,
		"exec",
		"-T",
		"redis",
		"redis-cli",
		"FLUSHALL",
	)

	resetDB := filepath.Join(root, "scripts", "reset-db.sql")

	runShell(fmt.Sprintf(
		"cat %s | docker compose -f %s exec -T postgres psql -U lab -d failurelab",
		resetDB,
		compose,
	))

	migrate := filepath.Join(root, "scripts", "migrate.sh")
	runShell(migrate)

	seed := filepath.Join(root, "scripts", "seed.sh")
	runShell(seed)

	fmt.Println("✓ Redis cleared")
	fmt.Println("✓ PostgreSQL reset")
	fmt.Println("✓ PostgreSQL migrated")
	fmt.Println("✓ PostgreSQL seeded")
	fmt.Println("✓ FailureLab reset")
}

func run(name string, args ...string) {
	fmt.Printf("→ %s %s\n\n", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		fmt.Printf("\n✗ command failed: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`
FailureLab

Usage:
  lab <command>

Commands:
  up                              Start local infrastructure
  down                            Stop local infrastructure
  status                          Show container status
  logs                            Follow container logs
  reset                           Reset disposable application state
  scenarios                       List available failure scenarios
  verify <scenario>               Verify a scenario
  reproduce <scenario>            Reproduce a failure scenario
`)
}
