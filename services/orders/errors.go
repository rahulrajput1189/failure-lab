package main

import "errors"

var (
	ErrInvalidUser       = errors.New("invalid user")
	ErrInvalidProduct    = errors.New("invalid product")
	ErrInsufficientStock = errors.New("insufficient stock")
)
