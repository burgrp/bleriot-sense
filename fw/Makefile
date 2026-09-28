.PHONY: build flash gdb rtt install-pack disassembly new hub

build:
	go run ./cmd/dev node build --name sense

flash:
	go run ./cmd/dev node build --name sense --flash --rtt

disassembly:
	go run ./cmd/dev node build --name sense --disassembly

gdb rtt install-pack:
	go run ./cmd/dev node $@ --name sense

new:
	go run ./cmd/dev node new

hub:
	go run ./cmd/dev hub --diagnostics rf
