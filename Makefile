.PHONY: build build-docker clean run-server run-host

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/tunnel-server ./cmd/server
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/tunnel-host   ./cmd/host

build-docker:
	docker build --output bin/ .

clean:
	rm -rf output/

run-server:
	go run cmd/server/server.go -tunnel :7000 -listen :8080

run-host:
	go run cmd/host/host.go -server 127.0.0.1:7000 -forward 127.0.0.1:3000
