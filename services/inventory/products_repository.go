package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrInventoryIdempotencyConflict = errors.New("inventory idempotency key conflict")

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

// ReleaseStockIdempotent releases stock once for a caller-provided operation key.
// The stock update and operation record commit atomically in PostgreSQL.
func (r *ProductRepository) ReleaseStockIdempotent(
	ctx context.Context,
	operationKey string,
	productID int64,
	quantity int,
) error {
	if operationKey == "" {
		return fmt.Errorf("operation key is required")
	}
	if quantity <= 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}

	beginner, ok := r.db.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return fmt.Errorf("database executor does not support transactions")
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin inventory release transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var insertedKey string
	err = tx.QueryRow(
		ctx,
		`INSERT INTO inventory_stock_operations
			(operation_key, product_id, action, quantity)
		 VALUES ($1, $2, 'release', $3)
		 ON CONFLICT (operation_key) DO NOTHING
		 RETURNING operation_key`,
		operationKey,
		productID,
		quantity,
	).Scan(&insertedKey)

	if errors.Is(err, pgx.ErrNoRows) {
		var existingProductID int64
		var existingAction string
		var existingQuantity int
		if err := tx.QueryRow(
			ctx,
			`SELECT product_id, action, quantity
			 FROM inventory_stock_operations
			 WHERE operation_key = $1`,
			operationKey,
		).Scan(&existingProductID, &existingAction, &existingQuantity); err != nil {
			return fmt.Errorf("load existing inventory operation: %w", err)
		}

		if existingProductID != productID || existingAction != "release" || existingQuantity != quantity {
			return fmt.Errorf("%w: key %q was already used for a different inventory operation", ErrInventoryIdempotencyConflict, operationKey)
		}

		return nil
	}
	if err != nil {
		return fmt.Errorf("record inventory release operation: %w", err)
	}

	result, err := tx.Exec(
		ctx,
		`UPDATE products SET stock = stock + $1 WHERE id = $2`,
		quantity,
		productID,
	)
	if err != nil {
		return fmt.Errorf("release inventory stock: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("product %d not found", productID)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit inventory release: %w", err)
	}
	return nil
}
