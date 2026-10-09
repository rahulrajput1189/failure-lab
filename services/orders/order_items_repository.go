package main

import (
	"context"
	// "github.com/jackc/pgx/v5"
)

type OrderItem struct {
	ID         int64
	OrderID    int64
	ProductID  int64
	Quantity   int
	PriceCents int
}

type OrderItemRepository struct {
	db DBExecutor
}

func NewOrderItemRepository(db DBExecutor) *OrderItemRepository {
	return &OrderItemRepository{db: db}
}

func (r *OrderItemRepository) Create(
	ctx context.Context,
	orderID int64,
	productID int64,
	quantity int,
	priceCents int,
) (*OrderItem, error) {
	item := &OrderItem{}

	err := r.db.QueryRow(
		ctx,
		`INSERT INTO order_items (
			order_id,
			product_id,
			quantity,
			price_cents
		)
		VALUES ($1, $2, $3, $4)
		RETURNING id, order_id, product_id, quantity, price_cents`,
		orderID,
		productID,
		quantity,
		priceCents,
	).Scan(
		&item.ID,
		&item.OrderID,
		&item.ProductID,
		&item.Quantity,
		&item.PriceCents,
	)

	if err != nil {
		return nil, err
	}

	return item, nil
}

func (r *OrderItemRepository) GetByOrderID(
	ctx context.Context,
	orderID int64,
) (*OrderItem, error) {
	item := &OrderItem{}

	err := r.db.QueryRow(
		ctx,
		`SELECT id, order_id, product_id, quantity, price_cents
		 FROM order_items
		 WHERE order_id = $1
		 ORDER BY id
		 LIMIT 1`,
		orderID,
	).Scan(
		&item.ID,
		&item.OrderID,
		&item.ProductID,
		&item.Quantity,
		&item.PriceCents,
	)

	if err != nil {
		return nil, err
	}

	return item, nil
}
