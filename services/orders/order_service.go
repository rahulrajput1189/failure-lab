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
	failAfterInventoryRelease bool
	paymentClient             *PaymentClient
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
		db:              db,
		userRepo:        userRepo,
		orderRepo:       orderRepo,
		orderItemRepo:   orderItemRepo,
		inventoryClient: inventoryClient,
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

	if s.paymentClient != nil {
		payment, err := s.paymentClient.CreatePayment(
			ctx,
			order.ID,
			order.TotalCents,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"order %d created but payment creation failed: %w",
				order.ID,
				err,
			)
		}

		if _, err := s.paymentClient.AuthorizePayment(ctx, payment.ID); err != nil {
			return nil, fmt.Errorf(
				"order %d created but payment authorization failed: %w",
				order.ID,
				err,
			)
		}

		order, err = s.orderRepo.Confirm(ctx, order.ID)
		if err != nil {
			return nil, fmt.Errorf(
				"payment authorized but order confirmation failed: %w",
				err,
			)
		}
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
	if s.inventoryClient == nil {
		return nil, fmt.Errorf("inventory client is not configured")
	}

	order, err := s.orderRepo.StartCancellation(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status == "CANCELLED" {
		return order, nil
	}

	orderItem, err := s.orderItemRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order item for cancellation: %w", err)
	}

	// Refund before releasing stock. If a later step fails, the order remains
	// CANCELLING; a retry observes REFUNDED and safely continues without a
	// second refund.
	if s.paymentClient != nil {
		if err := s.refundOrderPayment(ctx, orderID); err != nil {
			return nil, fmt.Errorf("failed to refund payment for order %d: %w", orderID, err)
		}
	}

	operationKey := fmt.Sprintf("order-cancel-release:%d", orderID)
	if _, err := s.inventoryClient.ReleaseStockWithKey(
		ctx,
		orderItem.ProductID,
		orderItem.Quantity,
		operationKey,
	); err != nil {
		return nil, fmt.Errorf("failed to release inventory for order %d: %w", orderID, err)
	}

	// This injection deliberately leaves the order in CANCELLING. A retry uses
	// the same inventory idempotency key, so it cannot release stock twice.
	if s.failAfterInventoryRelease {
		return nil, fmt.Errorf("failure injected after inventory release")
	}

	order, err = s.orderRepo.CompleteCancellation(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("inventory released but order cancellation remains pending: %w", err)
	}

	return order, nil
}

func (s *OrderService) refundOrderPayment(ctx context.Context, orderID int64) error {
	payment, err := s.paymentClient.GetPaymentForOrder(ctx, orderID)
	if errors.Is(err, ErrPaymentNotFound) {
		// Orders created before payment integration, or orders whose payment
		// creation never completed, have nothing to refund.
		return nil
	}
	if err != nil {
		return err
	}

	switch payment.Status {
	case PaymentStatusAuthorized:
		_, err = s.paymentClient.RefundPayment(ctx, payment.ID)
		return err
	case PaymentStatusRefunded, PaymentStatusPending, PaymentStatusFailed:
		// REFUNDED is an idempotent retry. PENDING and FAILED payments have
		// not been authorized, so no refund is required.
		return nil
	default:
		return fmt.Errorf("unsupported payment status %q for order %d", payment.Status, orderID)
	}
}

func (s *OrderService) SetPaymentClient(client *PaymentClient) {
	s.paymentClient = client
}
