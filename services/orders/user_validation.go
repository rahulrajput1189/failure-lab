package main

import (
	"context"
	"fmt"
)

func validateUser(
	ctx context.Context,
	repo *UserRepository,
	userID int64,
) error {
	exists, err := repo.Exists(ctx, userID)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf(
			"%w: user %d does not exist",
			ErrInvalidUser,
			userID,
		)
	}

	return nil
}
