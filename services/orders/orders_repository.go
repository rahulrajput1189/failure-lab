package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Order struct {
	ID         int64
	UserID     int64
	Status     string
	TotalCents int
}

type OrderRepository struct {
	db DBExecutor
}

func NewOrderRepository(db DBExecutor) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(
	ctx context.Context,
	userID int64,
	status string,
	totalCents int,
) (*Order, error) {
	order := &Order{}

	err := r.db.QueryRow(
		ctx,
		`INSERT INTO orders (user_id, status, total_cents)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, status, total_cents`,
		userID,
		status,
		totalCents,
	).Scan(
		&order.ID,
		&order.UserID,
		&order.Status,
		&order.TotalCents,
	)
	if err != nil {
		return nil, err
	}

	return order, nil
}

func (r *OrderRepository) GetByID(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	order := &Order{}

	err := r.db.QueryRow(
		ctx,
		`SELECT id, user_id, status, total_cents
		 FROM orders
		 WHERE id = $1`,
		orderID,
	).Scan(
		&order.ID,
		&order.UserID,
		&order.Status,
		&order.TotalCents,
	)
	if err != nil {
		return nil, err
	}

	return order, nil
}

func (r *OrderRepository) Confirm(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	order := &Order{}

	err := r.db.QueryRow(
		ctx,
		`UPDATE orders
		 SET status = 'CONFIRMED'
		 WHERE id = $1 AND status = 'PENDING'
		 RETURNING id, user_id, status, total_cents`,
		orderID,
	).Scan(
		&order.ID,
		&order.UserID,
		&order.Status,
		&order.TotalCents,
	)
	if err == nil {
		return order, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("confirm order: %w", err)
	}

	existing, lookupErr := r.GetByID(ctx, orderID)
	if lookupErr != nil {
		return nil, lookupErr
	}

	return nil, fmt.Errorf(
		"cannot confirm order %d from status %s: %w",
		orderID,
		existing.Status,
		ErrInvalidOrderState,
	)
}

// StartCancellation persists the saga's in-progress state before contacting
// Inventory. Repeated calls are safe while the order is CANCELLING or CANCELLED.
func (r *OrderRepository) StartCancellation(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	order := &Order{}
	err := r.db.QueryRow(
		ctx,
		`UPDATE orders
		 SET status = 'CANCELLING'
		 WHERE id = $1 AND status IN ('PENDING', 'CONFIRMED')
		 RETURNING id, user_id, status, total_cents`,
		orderID,
	).Scan(&order.ID, &order.UserID, &order.Status, &order.TotalCents)
	if err == nil {
		return order, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("start order cancellation: %w", err)
	}

	existing, lookupErr := r.GetByID(ctx, orderID)
	if lookupErr != nil {
		return nil, lookupErr
	}
	if existing.Status == "CANCELLING" || existing.Status == "CANCELLED" {
		return existing, nil
	}
	return nil, fmt.Errorf(
		"cannot start cancellation for order %d from status %s: %w",
		orderID,
		existing.Status,
		ErrInvalidOrderState,
	)
}

// CompleteCancellation finalizes a cancellation after Inventory confirms the
// idempotent release. It is safe to retry after a lost response.
func (r *OrderRepository) CompleteCancellation(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	order := &Order{}
	err := r.db.QueryRow(
		ctx,
		`UPDATE orders
		 SET status = 'CANCELLED'
		 WHERE id = $1 AND status = 'CANCELLING'
		 RETURNING id, user_id, status, total_cents`,
		orderID,
	).Scan(&order.ID, &order.UserID, &order.Status, &order.TotalCents)
	if err == nil {
		return order, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("complete order cancellation: %w", err)
	}

	existing, lookupErr := r.GetByID(ctx, orderID)
	if lookupErr != nil {
		return nil, lookupErr
	}
	if existing.Status == "CANCELLED" {
		return existing, nil
	}
	return nil, fmt.Errorf(
		"cannot complete cancellation for order %d from status %s: %w",
		orderID,
		existing.Status,
		ErrInvalidOrderState,
	)
}

func (r *OrderRepository) Cancel(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	order := &Order{}

	err := r.db.QueryRow(
		ctx,
		`UPDATE orders
		 SET status = 'CANCELLED'
		 WHERE id = $1 AND status = 'PENDING'
		 RETURNING id, user_id, status, total_cents`,
		orderID,
	).Scan(
		&order.ID,
		&order.UserID,
		&order.Status,
		&order.TotalCents,
	)
	if err == nil {
		return order, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	existing, lookupErr := r.GetByID(ctx, orderID)
	if lookupErr != nil {
		return nil, lookupErr
	}

	return nil, fmt.Errorf(
		"cannot cancel order %d from status %s: %w",
		orderID,
		existing.Status,
		ErrInvalidOrderState,
	)
}
