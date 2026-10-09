package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Scenario struct {
	Name        string
	Description string
	Reproduce   func() error
	Verify      func() error
}

var scenarios = map[string]Scenario{
	"inventory-reservation-leak": {
		Name:        "inventory-reservation-leak",
		Description: "Inventory remains reserved after the order transaction rolls back.",
		Reproduce:   reproduceInventoryReservationLeak,
		Verify:      verifyInventoryReservationLeak,
	},
	"order-cancellation-inventory-leak": {
		Name:        "order-cancellation-inventory-leak",
		Description: "Cancelling an order leaves its inventory reservation unreleased.",
		Reproduce:   reproduceOrderCancellationInventoryLeak,
		Verify:      verifyOrderCancellationInventoryLeak,
	},
	"inventory-release-order-rollback": {
		Name:        "inventory-release-order-rollback",
		Description: "Inventory is released but the order cancellation transaction rolls back.",
		Reproduce:   reproduceInventoryReleaseOrderRollback,
		Verify:      verifyInventoryReleaseOrderRollback,
	},
}

func getScenario(name string) Scenario {
	scenario, ok := scenarios[name]

	if !ok {
		fmt.Printf("unknown scenario: %s\n\n", name)
		fmt.Println("Available scenarios:")

		for name := range scenarios {
			fmt.Printf("  %s\n", name)
		}

		os.Exit(1)
	}

	return scenario
}

func verifyInventoryReservationLeak() error {
	fmt.Println("→ verifying inventory-reservation-leak")
	fmt.Println()

	stock, err := queryInventoryStock(1)
	if err != nil {
		return err
	}

	fmt.Printf("Inventory stock: %d\n", stock)

	if stock != 8 {
		return fmt.Errorf(
			"expected inventory stock to be 8 after leaked reservation, got %d",
			stock,
		)
	}

	orderCount, err := queryPostgres(
		"SELECT COUNT(*) FROM orders",
	)
	if err != nil {
		return err
	}

	fmt.Printf("Orders: %s\n", orderCount)

	if strings.TrimSpace(orderCount) != "0" {
		return fmt.Errorf(
			"expected 0 orders after rollback, got %s",
			orderCount,
		)
	}

	orderItemCount, err := queryPostgres(
		"SELECT COUNT(*) FROM order_items",
	)
	if err != nil {
		return err
	}

	fmt.Printf("Order items: %s\n", orderItemCount)

	if strings.TrimSpace(orderItemCount) != "0" {
		return fmt.Errorf(
			"expected 0 order items after rollback, got %s",
			orderItemCount,
		)
	}

	fmt.Println()
	fmt.Println("✓ inventory reservation leaked")
	fmt.Println("✓ orders transaction rolled back")
	fmt.Println("✓ order item was rolled back")
	fmt.Println()
	fmt.Println("FAILURE REPRODUCED")

	return nil
}

func verifyInventoryReleaseOrderRollback() error {
	fmt.Println("→ verifying inventory-release-order-rollback")
	fmt.Println()

	stock, err := queryInventoryStock(1)
	if err != nil {
		return err
	}

	fmt.Printf("Inventory stock: %d\n", stock)

	if stock != 10 {
		return fmt.Errorf(
			"expected inventory stock to return to 10, got %d",
			stock,
		)
	}

	orderStatus, err := queryPostgres(
		"SELECT status FROM orders WHERE id = 1",
	)
	if err != nil {
		return err
	}

	fmt.Printf("Order status: %s\n", orderStatus)

	if strings.TrimSpace(orderStatus) != "PENDING" {
		return fmt.Errorf(
			"expected order to remain PENDING after rollback, got %s",
			orderStatus,
		)
	}

	fmt.Println()
	fmt.Println("✓ inventory was released")
	fmt.Println("✓ order cancellation transaction rolled back")
	fmt.Println("✓ order remains PENDING")
	fmt.Println()
	fmt.Println("FAILURE REPRODUCED")

	return nil
}

func queryInventoryStock(productID int) (int, error) {
	cmd := exec.Command(
		"curl",
		"-s",
		fmt.Sprintf(
			"http://localhost:8082/products/%d",
			productID,
		),
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf(
			"failed to query inventory: %w",
			err,
		)
	}

	response := string(output)

	var stock int

	_, err = fmt.Sscanf(
		response,
		`{"ID":1,"Name":"Laptop","PriceCents":100000,"Stock":%d}`,
		&stock,
	)

	if err != nil {
		return 0, fmt.Errorf(
			"failed to parse inventory response: %s",
			response,
		)
	}

	return stock, nil
}

func queryPostgres(query string) (string, error) {
	compose := composeFile()

	cmd := exec.Command(
		"docker",
		"compose",
		"-f",
		compose,
		"exec",
		"-T",
		"postgres",
		"psql",
		"-U",
		"lab",
		"-d",
		"failurelab",
		"-At",
		"-c",
		query,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(
			"failed to query PostgreSQL: %w",
			err,
		)
	}

	return strings.TrimSpace(string(output)), nil
}

func listScenarios() {
	fmt.Println("Available scenarios:")
	fmt.Println()

	for _, scenario := range scenarios {
		fmt.Printf("%s\n", scenario.Name)
		fmt.Printf("  %s\n", scenario.Description)
		fmt.Println()
	}
}
