
.PHONY: build test clean

build:
	mkdir -p bin
	go build -o bin/lab ./cmd/lab

test:
	go test ./...

clean:
	rm -rf bin
