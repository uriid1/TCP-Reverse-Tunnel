.PHONY: build clean run-server run-host

build:
	go build -o bin/server cmd/server/server.go
	go build -o bin/host cmd/host/host.go

clean:
	rm -rf output/

run-server:
	go run cmd/server/server.go -tunnel :7000 -listen :8080

run-host:
	go run cmd/host/host.go -server 127.0.0.1:7000 -forward 127.0.0.1:3000
