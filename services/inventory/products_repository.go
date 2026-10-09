package main

import (
	"context"
	"fmt"
)

type Product struct {
	ID         int64
	Name       string
	PriceCents int
	Stock      int
}

type ProductRepository struct {
	db DBExecutor
}

func NewProductRepository(db DBExecutor) *ProductRepository {
	return &ProductRepository{
		db: db,
	}
}

func (r *ProductRepository) GetByID(
	ctx context.Context,
	productID int64,
) (*Product, error) {
	product := &Product{}

	err := r.db.QueryRow(
		ctx,
		`SELECT id, name, price_cents, stock
		 FROM products
		 WHERE id = $1`,
		productID,
	).Scan(
		&product.ID,
		&product.Name,
		&product.PriceCents,
		&product.Stock,
	)

	if err != nil {
		return nil, err
	}

	return product, nil
}

func (r *ProductRepository) ReserveStock(
	ctx context.Context,
	productID int64,
	quantity int,
) error {
	result, err := r.db.Exec(
		ctx,
		`UPDATE products
		 SET stock = stock - $1
		 WHERE id = $2
		   AND stock >= $1`,
		quantity,
		productID,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf(
			"insufficient stock for product %d",
			productID,
		)
	}

	return nil
}

func (r *ProductRepository) ReleaseStock(
	ctx context.Context,
	productID int64,
	quantity int,
) error {
	result, err := r.db.Exec(
		ctx,
		`UPDATE products
		 SET stock = stock + $1
		 WHERE id = $2`,
		quantity,
		productID,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf(
			"product %d not found",
			productID,
		)
	}

	return nil
}
