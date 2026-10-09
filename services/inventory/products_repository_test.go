package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newInventoryTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	config := loadConfig()
	db := newDatabasePool(config)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func resetInventoryProduct(t *testing.T, db *pgxpool.Pool) {
	t.Helper()

	_, err := db.Exec(
		context.Background(),
		`UPDATE products
		 SET stock = 10
		 WHERE id = 1`,
	)

	if err != nil {
		t.Fatalf("failed to reset product stock: %v", err)
	}
}

func TestProductRepositoryGetByID(t *testing.T) {
	db := newInventoryTestDB(t)
	resetInventoryProduct(t, db)

	repo := NewProductRepository(db)

	product, err := repo.GetByID(
		context.Background(),
		1,
	)

	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if product.ID != 1 {
		t.Fatalf("expected product ID 1, got %d", product.ID)
	}

	if product.Name != "Laptop" {
		t.Fatalf(
			"expected product name Laptop, got %s",
			product.Name,
		)
	}

	if product.PriceCents != 100000 {
		t.Fatalf(
			"expected price 100000, got %d",
			product.PriceCents,
		)
	}

	if product.Stock != 10 {
		t.Fatalf(
			"expected stock 10, got %d",
			product.Stock,
		)
	}
}

func TestProductRepositoryReserveStock(t *testing.T) {
	db := newInventoryTestDB(t)
	resetInventoryProduct(t, db)

	repo := NewProductRepository(db)

	err := repo.ReserveStock(
		context.Background(),
		1,
		3,
	)

	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	product, err := repo.GetByID(
		context.Background(),
		1,
	)

	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if product.Stock != 7 {
		t.Fatalf(
			"expected stock 7 after reservation, got %d",
			product.Stock,
		)
	}

	resetInventoryProduct(t, db)
}

func TestProductRepositoryReserveStockInsufficientStock(t *testing.T) {
	db := newInventoryTestDB(t)
	resetInventoryProduct(t, db)

	repo := NewProductRepository(db)

	err := repo.ReserveStock(
		context.Background(),
		1,
		11,
	)

	if err == nil {
		t.Fatal("expected insufficient stock error")
	}

	product, err := repo.GetByID(
		context.Background(),
		1,
	)

	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if product.Stock != 10 {
		t.Fatalf(
			"expected stock to remain 10, got %d",
			product.Stock,
		)
	}
}

func TestProductRepositoryReleaseStock(t *testing.T) {
	db := newInventoryTestDB(t)
	resetInventoryProduct(t, db)

	repo := NewProductRepository(db)

	err := repo.ReserveStock(
		context.Background(),
		1,
		3,
	)

	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	err = repo.ReleaseStock(
		context.Background(),
		1,
		3,
	)

	if err != nil {
		t.Fatalf("ReleaseStock failed: %v", err)
	}

	product, err := repo.GetByID(
		context.Background(),
		1,
	)

	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if product.Stock != 10 {
		t.Fatalf(
			"expected stock 10 after release, got %d",
			product.Stock,
		)
	}

	resetInventoryProduct(t, db)
}
