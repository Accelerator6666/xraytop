.PHONY: build test vet
build:
	go build -o xraytop ./cmd/xraytop
test:
	go test ./...
vet:
	go vet ./...
