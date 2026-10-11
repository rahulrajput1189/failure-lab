package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	PaymentStatusPending    = "PENDING"
	PaymentStatusAuthorized = "AUTHORIZED"
	PaymentStatusRefunded   = "REFUNDED"
	PaymentStatusFailed     = "FAILED"
)

var (
	ErrPaymentNotFound     = errors.New("payment not found")
	ErrInvalidPaymentState = errors.New("invalid payment state transition")
)

type Payment struct {
	ID          int64
	OrderID     int64
	Status      string
	AmountCents int
}

type PaymentRepository struct {
	db DBExecutor
}

func NewPaymentRepository(db DBExecutor) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(
	ctx context.Context,
	orderID int64,
	amountCents int,
) (*Payment, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order ID must be greater than zero")
	}

	if amountCents <= 0 {
		return nil, fmt.Errorf("payment amount must be greater than zero")
	}

	payment := &Payment{}

	err := r.db.QueryRow(
		ctx,
		`INSERT INTO payments (order_id, status, amount_cents)
		 VALUES ($1, $2, $3)
		 RETURNING id, order_id, status, amount_cents`,
		orderID,
		PaymentStatusPending,
		amountCents,
	).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.Status,
		&payment.AmountCents,
	)
	if err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}

	return payment, nil
}

func (r *PaymentRepository) GetByID(
	ctx context.Context,
	paymentID int64,
) (*Payment, error) {
	if paymentID <= 0 {
		return nil, fmt.Errorf("payment ID must be greater than zero")
	}

	payment := &Payment{}

	err := r.db.QueryRow(
		ctx,
		`SELECT id, order_id, status, amount_cents
		 FROM payments
		 WHERE id = $1`,
		paymentID,
	).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.Status,
		&payment.AmountCents,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get payment: %w", ErrPaymentNotFound)
		}
		return nil, fmt.Errorf("get payment: %w", err)
	}

	return payment, nil
}

func (r *PaymentRepository) GetByOrderID(
	ctx context.Context,
	orderID int64,
) (*Payment, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order ID must be greater than zero")
	}

	payment := &Payment{}

	err := r.db.QueryRow(
		ctx,
		`SELECT id, order_id, status, amount_cents
		 FROM payments
		 WHERE order_id = $1
		 ORDER BY id
		 LIMIT 1`,
		orderID,
	).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.Status,
		&payment.AmountCents,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get payment by order: %w", ErrPaymentNotFound)
		}
		return nil, fmt.Errorf("get payment by order: %w", err)
	}

	return payment, nil
}

func (r *PaymentRepository) Authorize(
	ctx context.Context,
	paymentID int64,
) (*Payment, error) {
	return r.transition(
		ctx,
		paymentID,
		PaymentStatusPending,
		PaymentStatusAuthorized,
	)
}

func (r *PaymentRepository) Refund(
	ctx context.Context,
	paymentID int64,
) (*Payment, error) {
	return r.transition(
		ctx,
		paymentID,
		PaymentStatusAuthorized,
		PaymentStatusRefunded,
	)
}

func (r *PaymentRepository) transition(
	ctx context.Context,
	paymentID int64,
	expectedStatus string,
	newStatus string,
) (*Payment, error) {
	if paymentID <= 0 {
		return nil, fmt.Errorf("payment ID must be greater than zero")
	}

	payment := &Payment{}

	err := r.db.QueryRow(
		ctx,
		`UPDATE payments
		 SET status = $1
		 WHERE id = $2 AND status = $3
		 RETURNING id, order_id, status, amount_cents`,
		newStatus,
		paymentID,
		expectedStatus,
	).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.Status,
		&payment.AmountCents,
	)
	if err == nil {
		return payment, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("transition payment status: %w", err)
	}

	existingPayment, lookupErr := r.GetByID(ctx, paymentID)
	if lookupErr != nil {
		return nil, lookupErr
	}

	return nil, fmt.Errorf(
		"payment %d cannot transition from %s to %s (current status: %s): %w",
		paymentID,
		expectedStatus,
		newStatus,
		existingPayment.Status,
		ErrInvalidPaymentState,
	)
}
