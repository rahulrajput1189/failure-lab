.PHONY: build install test

build:
	go build -o bin/lab ./cmd/lab

install: build
	mkdir -p $(HOME)/bin
	cp bin/lab $(HOME)/bin/lab

test:
	go test ./...