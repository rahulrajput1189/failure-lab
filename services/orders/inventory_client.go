package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type InventoryClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewInventoryClient(
	baseURL string,
	httpClient *http.Client,
) *InventoryClient {
	return &InventoryClient{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

type InventoryProduct struct {
	ID         int64
	Name       string
	PriceCents int
	Stock      int
}

type ReserveStockRequest struct {
	Quantity int `json:"quantity"`
}

func (c *InventoryClient) GetProduct(
	ctx context.Context,
	productID int64,
) (*InventoryProduct, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/products/%d", c.baseURL, productID),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create inventory request: %w",
			err,
		)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"inventory request failed: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf(
			"%w: product %d does not exist",
			ErrInvalidProduct,
			productID,
		)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"inventory returned status %d",
			resp.StatusCode,
		)
	}

	var product InventoryProduct

	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		return nil, fmt.Errorf(
			"failed to decode inventory response: %w",
			err,
		)
	}

	return &product, nil
}

func (c *InventoryClient) ReserveStock(
	ctx context.Context,
	productID int64,
	quantity int,
) (*InventoryProduct, error) {
	return c.changeStock(ctx, productID, quantity, "reserve")
}

func (c *InventoryClient) ReleaseStock(
	ctx context.Context,
	productID int64,
	quantity int,
) (*InventoryProduct, error) {
	return c.changeStock(ctx, productID, quantity, "release")
}

func (c *InventoryClient) changeStock(
	ctx context.Context,
	productID int64,
	quantity int,
	action string,
) (*InventoryProduct, error) {
	body, err := json.Marshal(ReserveStockRequest{
		Quantity: quantity,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to encode inventory request: %w",
			err,
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf(
			"%s/products/%d/%s",
			c.baseURL,
			productID,
			action,
		),
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create inventory request: %w",
			err,
		)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"inventory request failed: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf(
			"%w: product %d does not exist",
			ErrInvalidProduct,
			productID,
		)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"inventory returned status %d",
			resp.StatusCode,
		)
	}

	var product InventoryProduct

	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		return nil, fmt.Errorf(
			"failed to decode inventory response: %w",
			err,
		)
	}

	return &product, nil
}
