package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type reproducedOrder struct {
	ID int64 `json:"ID"`
}

func reproduceInventoryReservationLeak() error {
	fmt.Println("→ reproducing inventory-reservation-leak")
	fmt.Println()

	fmt.Println("→ resetting FailureLab")
	reset()

	fmt.Println()
	fmt.Println("→ starting Orders service with failure injection")

	stopOrdersService()

	if err := startOrdersServiceWithEnvironment(
		"FAILURELAB_FAIL_AFTER_INVENTORY_RESERVATION=true",
	); err != nil {
		return err
	}

	if err := waitForOrdersService(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("→ sending failing order request")

	statusCode, _, err := createOrderRequest()
	if err != nil {
		return err
	}

	fmt.Printf("HTTP status: %s\n", statusCode)

	if statusCode != "500" {
		return fmt.Errorf(
			"expected HTTP 500 from injected failure, got %s",
			statusCode,
		)
	}

	fmt.Println("✓ injected failure triggered")

	fmt.Println()
	fmt.Println("→ verifying scenario")

	return verifyInventoryReservationLeak()
}

func reproduceOrderCancellationInventoryLeak() error {
	fmt.Println("→ reproducing order-cancellation-inventory-leak")
	fmt.Println()

	fmt.Println("→ resetting FailureLab")
	reset()

	fmt.Println()
	fmt.Println("→ starting Orders service without failure injection")

	stopOrdersService()

	if err := startOrdersServiceWithEnvironment(); err != nil {
		return err
	}

	if err := waitForOrdersService(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("→ creating order")

	statusCode, response, err := createOrderRequest()
	if err != nil {
		return err
	}

	fmt.Printf("HTTP status: %s\n", statusCode)
	fmt.Printf("Response: %s\n", response)

	if statusCode != "201" {
		return fmt.Errorf(
			"expected HTTP 201 when creating order, got %s",
			statusCode,
		)
	}

	var order reproducedOrder

	if err := json.Unmarshal(
		[]byte(response),
		&order,
	); err != nil {
		return fmt.Errorf(
			"failed to decode created order: %w",
			err,
		)
	}

	fmt.Printf("✓ order created: %d\n", order.ID)

	fmt.Println()
	fmt.Println("→ cancelling order")

	cancelStatus, cancelResponse, err := cancelOrderRequest(order.ID)
	if err != nil {
		return err
	}

	fmt.Printf("HTTP status: %s\n", cancelStatus)
	fmt.Printf("Response: %s\n", cancelResponse)

	if cancelStatus != "200" {
		return fmt.Errorf(
			"expected HTTP 200 when cancelling order, got %s",
			cancelStatus,
		)
	}

	fmt.Println("✓ order cancelled")

	fmt.Println()
	fmt.Println("→ verifying scenario")

	return verifyOrderCancellationInventoryLeak()
}

func reproduceInventoryReleaseOrderRollback() error {
	fmt.Println("→ reproducing inventory-release-order-rollback")
	fmt.Println()

	fmt.Println("→ resetting FailureLab")
	reset()

	fmt.Println()
	fmt.Println("→ starting Orders service with failure injection")

	stopOrdersService()

	if err := startOrdersServiceWithEnvironment(
		"FAILURELAB_FAIL_AFTER_INVENTORY_RELEASE=true",
	); err != nil {
		return err
	}

	if err := waitForOrdersService(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("→ creating order")

	statusCode, response, err := createOrderRequest()
	if err != nil {
		return err
	}

	fmt.Printf("HTTP status: %s\n", statusCode)
	fmt.Printf("Response: %s\n", response)

	if statusCode != "201" {
		return fmt.Errorf(
			"expected HTTP 201 when creating order, got %s",
			statusCode,
		)
	}

	var order reproducedOrder

	if err := json.Unmarshal([]byte(response), &order); err != nil {
		return fmt.Errorf(
			"failed to decode created order: %w",
			err,
		)
	}

	fmt.Printf("✓ order created: %d\n", order.ID)

	fmt.Println()
	fmt.Println("→ cancelling order with injected failure")

	cancelStatus, cancelResponse, err := cancelOrderRequest(order.ID)
	if err != nil {
		return err
	}

	fmt.Printf("HTTP status: %s\n", cancelStatus)
	fmt.Printf("Response: %s\n", cancelResponse)

	if cancelStatus != "500" {
		return fmt.Errorf(
			"expected HTTP 500 from injected failure, got %s",
			cancelStatus,
		)
	}

	fmt.Println("✓ injected failure triggered")

	fmt.Println()
	fmt.Println("→ verifying scenario")

	return verifyInventoryReleaseOrderRollback()
}

func createOrderRequest() (string, string, error) {
	cmd := exec.Command(
		"curl",
		"-s",
		"-X",
		"POST",
		"http://localhost:8081/orders",
		"-H",
		"Content-Type: application/json",
		"-d",
		`{"user_id":1,"product_id":1,"quantity":2}`,
		"-w",
		"\n%{http_code}",
	)

	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf(
			"failed to create order request: %w",
			err,
		)
	}

	lines := strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	)

	if len(lines) < 2 {
		return "", "", fmt.Errorf(
			"unexpected order response: %s",
			string(output),
		)
	}

	statusCode := lines[len(lines)-1]
	response := strings.Join(lines[:len(lines)-1], "\n")

	return statusCode, response, nil
}

func cancelOrderRequest(orderID int64) (string, string, error) {
	cmd := exec.Command(
		"curl",
		"-s",
		"-X",
		"DELETE",
		fmt.Sprintf(
			"http://localhost:8081/orders/%d",
			orderID,
		),
		"-w",
		"\n%{http_code}",
	)

	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf(
			"failed to cancel order: %w",
			err,
		)
	}

	lines := strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	)

	if len(lines) < 2 {
		return "", "", fmt.Errorf(
			"unexpected cancellation response: %s",
			string(output),
		)
	}

	statusCode := lines[len(lines)-1]
	response := strings.Join(lines[:len(lines)-1], "\n")

	return statusCode, response, nil
}

func verifyOrderCancellationInventoryLeak() error {
	fmt.Println("→ verifying order-cancellation-inventory-leak")
	fmt.Println()

	stock, err := queryInventoryStock(1)
	if err != nil {
		return err
	}

	fmt.Printf("Inventory stock: %d\n", stock)

	if stock != 10 {
		return fmt.Errorf(
			"expected inventory stock to return to 10 after cancellation, got %d",
			stock,
		)
	}

	orderCount, err := queryPostgres(
		"SELECT COUNT(*) FROM orders WHERE status = 'CANCELLED'",
	)
	if err != nil {
		return err
	}

	fmt.Printf("Cancelled orders: %s\n", orderCount)

	if strings.TrimSpace(orderCount) != "1" {
		return fmt.Errorf(
			"expected 1 cancelled order, got %s",
			orderCount,
		)
	}

	fmt.Println()
	fmt.Println("✓ order was cancelled")
	fmt.Println("✓ inventory reservation was released")
	fmt.Println("✓ inventory stock was restored")
	fmt.Println()
	fmt.Println("FIX VERIFIED")

	return nil
}

func stopOrdersService() {
	cmd := exec.Command(
		"sh",
		"-c",
		`lsof -ti tcp:8081 | xargs -r kill 2>/dev/null || true`,
	)

	_ = cmd.Run()

	time.Sleep(500 * time.Millisecond)
}

func startOrdersServiceWithEnvironment(
	extraEnvironment ...string,
) error {
	root := projectRoot()

	logFile, err := os.OpenFile(
		"/tmp/failurelab-orders.log",
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0644,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to create Orders log: %w",
			err,
		)
	}

	cmd := exec.Command(
		"go",
		"run",
		"./services/orders",
	)

	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, extraEnvironment...)

	if err := cmd.Start(); err != nil {
		logFile.Close()

		return fmt.Errorf(
			"failed to start Orders service: %w",
			err,
		)
	}

	logFile.Close()

	return nil
}

func waitForOrdersService() error {
	for i := 0; i < 30; i++ {
		cmd := exec.Command(
			"curl",
			"-s",
			"-o",
			"/dev/null",
			"-w",
			"%{http_code}",
			"http://localhost:8081/health",
		)

		output, err := cmd.Output()

		if err == nil && string(output) == "200" {
			fmt.Println("✓ Orders service is ready")
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("Orders service failed to become ready.")
	fmt.Println()
	fmt.Println("Orders service log:")
	fmt.Println("-------------------")

	log, err := os.ReadFile("/tmp/failurelab-orders.log")
	if err == nil {
		fmt.Print(string(log))
	} else {
		fmt.Printf("could not read Orders log: %v\n", err)
	}

	return fmt.Errorf(
		"Orders service did not become ready",
	)
}

func parseStatusCode(value string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(value))
}
