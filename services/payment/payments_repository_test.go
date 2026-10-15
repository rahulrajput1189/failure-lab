package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newPaymentTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	db := newDatabasePool(loadConfig())

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func createPaymentForTest(t *testing.T, repo *PaymentRepository) *Payment {
	t.Helper()

	ctx := context.Background()
	var orderID int64

	err := repo.db.QueryRow(
		ctx,
		`INSERT INTO orders (user_id, status, total_cents)
		 SELECT id, 'PENDING', 2500
		 FROM users
		 ORDER BY id
		 LIMIT 1
		 RETURNING id`,
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("failed to create test order: %v", err)
	}

	t.Cleanup(func() {
		_, err := repo.db.Exec(
			ctx,
			`DELETE FROM orders WHERE id = $1`,
			orderID,
		)
		if err != nil {
			t.Errorf("failed to clean up test order: %v", err)
		}
	})

	payment, err := repo.Create(ctx, orderID, 2500)
	if err != nil {
		t.Fatalf("failed to create test payment: %v", err)
	}

	t.Cleanup(func() {
		_, err := repo.db.Exec(
			ctx,
			`DELETE FROM payments WHERE id = $1`,
			payment.ID,
		)
		if err != nil {
			t.Errorf("failed to clean up test payment: %v", err)
		}
	})

	return payment
}

func TestPaymentRepositoryCreate(t *testing.T) {
	db := newPaymentTestDB(t)
	repo := NewPaymentRepository(db)

	payment := createPaymentForTest(t, repo)

	if payment.ID <= 0 {
		t.Fatalf("expected a generated payment ID, got %d", payment.ID)
	}
	if payment.OrderID <= 0 {
		t.Fatalf("expected a valid order ID, got %d", payment.OrderID)
	}
	if payment.Status != PaymentStatusPending {
		t.Fatalf("expected status %s, got %s", PaymentStatusPending, payment.Status)
	}
	if payment.AmountCents != 2500 {
		t.Fatalf("expected amount 2500, got %d", payment.AmountCents)
	}
}

func TestPaymentRepositoryAuthorize(t *testing.T) {
	db := newPaymentTestDB(t)
	repo := NewPaymentRepository(db)
	created := createPaymentForTest(t, repo)

	payment, err := repo.Authorize(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}
	if payment.Status != PaymentStatusAuthorized {
		t.Fatalf("expected status %s, got %s", PaymentStatusAuthorized, payment.Status)
	}
}

func TestPaymentRepositoryRefund(t *testing.T) {
	db := newPaymentTestDB(t)
	repo := NewPaymentRepository(db)
	created := createPaymentForTest(t, repo)

	if _, err := repo.Authorize(context.Background(), created.ID); err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}

	payment, err := repo.Refund(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Refund failed: %v", err)
	}
	if payment.Status != PaymentStatusRefunded {
		t.Fatalf("expected status %s, got %s", PaymentStatusRefunded, payment.Status)
	}
}

func TestPaymentRepositoryRejectsInvalidTransitions(t *testing.T) {
	tests := []struct {
		name       string
		transition func(context.Context, *PaymentRepository, int64) (*Payment, error)
	}{
		{
			name: "refund pending payment",
			transition: func(ctx context.Context, repo *PaymentRepository, id int64) (*Payment, error) {
				return repo.Refund(ctx, id)
			},
		},
		{
			name: "authorize twice",
			transition: func(ctx context.Context, repo *PaymentRepository, id int64) (*Payment, error) {
				if _, err := repo.Authorize(ctx, id); err != nil {
					return nil, err
				}
				return repo.Authorize(ctx, id)
			},
		},
		{
			name: "refund twice",
			transition: func(ctx context.Context, repo *PaymentRepository, id int64) (*Payment, error) {
				if _, err := repo.Authorize(ctx, id); err != nil {
					return nil, err
				}
				if _, err := repo.Refund(ctx, id); err != nil {
					return nil, err
				}
				return repo.Refund(ctx, id)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPaymentTestDB(t)
			repo := NewPaymentRepository(db)
			created := createPaymentForTest(t, repo)

			_, err := tt.transition(context.Background(), repo, created.ID)
			if !errors.Is(err, ErrInvalidPaymentState) {
				t.Fatalf("expected ErrInvalidPaymentState, got %v", err)
			}
		})
	}
}

func TestPaymentRepositoryNotFound(t *testing.T) {
	db := newPaymentTestDB(t)
	repo := NewPaymentRepository(db)

	_, err := repo.Authorize(context.Background(), 9223372036854775807)
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}
}

func TestPaymentRepositoryRejectsInvalidID(t *testing.T) {
	db := newPaymentTestDB(t)
	repo := NewPaymentRepository(db)

	_, err := repo.Authorize(context.Background(), 0)
	if err == nil {
		t.Fatal("expected an error for payment ID zero")
	}

	_, err = repo.Refund(context.Background(), -1)
	if err == nil {
		t.Fatal("expected an error for negative payment ID")
	}
}
