package main

import "context"

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

func (r *OrderRepository) Cancel(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	order := &Order{}

	err := r.db.QueryRow(
		ctx,
		`UPDATE orders
		 SET status = 'CANCELLED'
		 WHERE id = $1
		   AND status = 'PENDING'
		 RETURNING id, user_id, status, total_cents`,
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
