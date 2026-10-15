package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Payment struct {
	ID          int64
	OrderID     int64
	Status      string
	AmountCents int
}

type PaymentClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPaymentClient(baseURL string, httpClient *http.Client) *PaymentClient {
	return &PaymentClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *PaymentClient) CreatePayment(
	ctx context.Context,
	orderID int64,
	amountCents int,
) (*Payment, error) {
	body, err := json.Marshal(map[string]int64{
		"order_id":     orderID,
		"amount_cents": int64(amountCents),
	})
	if err != nil {
		return nil, fmt.Errorf("encode payment request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/payments",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build payment request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create payment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"payment creation returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var payment Payment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		return nil, fmt.Errorf("decode payment response: %w", err)
	}

	return &payment, nil
}

func (c *PaymentClient) AuthorizePayment(
	ctx context.Context,
	paymentID int64,
) (*Payment, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/payments/%d/authorize", c.baseURL, paymentID),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build payment authorization request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authorize payment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"payment authorization returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var payment Payment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		return nil, fmt.Errorf("decode payment authorization response: %w", err)
	}

	return &payment, nil
}
