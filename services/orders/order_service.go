package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderService struct {
	db                        *pgxpool.Pool
	userRepo                  *UserRepository
	orderRepo                 *OrderRepository
	orderItemRepo             *OrderItemRepository
	inventoryClient           *InventoryClient
	failAfterInventoryReserve bool
}

func NewOrderService(
	db *pgxpool.Pool,
	userRepo *UserRepository,
	orderRepo *OrderRepository,
	orderItemRepo *OrderItemRepository,
	inventoryClients ...*InventoryClient,
) *OrderService {
	var inventoryClient *InventoryClient

	if len(inventoryClients) > 0 {
		inventoryClient = inventoryClients[0]
	}

	return &OrderService{
		db:                        db,
		userRepo:                  userRepo,
		orderRepo:                 orderRepo,
		orderItemRepo:             orderItemRepo,
		inventoryClient:           inventoryClient,
		failAfterInventoryReserve: false,
	}
}

type CreateOrderInput struct {
	UserID    int64
	ProductID int64
	Quantity  int
}

func (s *OrderService) CreateOrder(
	ctx context.Context,
	input CreateOrderInput,
) (*Order, error) {
	if err := validateUser(
		ctx,
		s.userRepo,
		input.UserID,
	); err != nil {
		return nil, err
	}

	if s.inventoryClient == nil {
		return nil, fmt.Errorf(
			"inventory client is not configured",
		)
	}

	product, err := s.inventoryClient.GetProduct(
		ctx,
		input.ProductID,
	)
	if err != nil {
		if errors.Is(err, ErrInvalidProduct) {
			return nil, err
		}

		return nil, fmt.Errorf(
			"failed to get product from inventory: %w",
			err,
		)
	}

	if input.Quantity <= 0 {
		return nil, fmt.Errorf(
			"%w: quantity must be greater than zero",
			ErrInsufficientStock,
		)
	}

	if input.Quantity > product.Stock {
		return nil, fmt.Errorf(
			"%w: product %d: requested %d, available %d",
			ErrInsufficientStock,
			product.ID,
			input.Quantity,
			product.Stock,
		)
	}

	tx, err := beginTransaction(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin transaction: %w",
			err,
		)
	}

	orderRepo := NewOrderRepository(tx)
	orderItemRepo := NewOrderItemRepository(tx)

	totalCents := product.PriceCents * input.Quantity

	order, err := orderRepo.Create(
		ctx,
		input.UserID,
		"PENDING",
		totalCents,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf(
			"failed to create order: %w",
			err,
		)
	}

	_, err = s.inventoryClient.ReserveStock(
		ctx,
		input.ProductID,
		input.Quantity,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf(
			"failed to reserve inventory stock: %w",
			err,
		)
	}

	if s.failAfterInventoryReserve {
		tx.Rollback(ctx)

		return nil, fmt.Errorf(
			"failure injected after inventory reservation",
		)
	}

	_, err = orderItemRepo.Create(
		ctx,
		order.ID,
		input.ProductID,
		input.Quantity,
		product.PriceCents,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf(
			"failed to create order item: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit order transaction: %w",
			err,
		)
	}

	return order, nil
}

func (s *OrderService) GetOrder(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	return s.orderRepo.GetByID(ctx, orderID)
}

func (s *OrderService) CancelOrder(
	ctx context.Context,
	orderID int64,
) (*Order, error) {
	tx, err := beginTransaction(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin cancellation transaction: %w",
			err,
		)
	}

	orderRepo := NewOrderRepository(tx)
	orderItemRepo := NewOrderItemRepository(tx)

	order, err := orderRepo.Cancel(ctx, orderID)
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}

	orderItem, err := orderItemRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf(
			"failed to get order item: %w",
			err,
		)
	}

	_, err = s.inventoryClient.ReleaseStock(
		ctx,
		orderItem.ProductID,
		orderItem.Quantity,
	)
	if err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf(
			"failed to release inventory for cancelled order: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit cancellation transaction: %w",
			err,
		)
	}

	return order, nil
}
