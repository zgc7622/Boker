.PHONY: build cluster test

default: build

build:
	go build -o bin/swarm    github.com/ethereum/chain/cmd/swarm
	go build -o bin/geth     github.com/ethereum/chain/cmd/geth
	go build -o bin/bootnode github.com/ethereum/chain/cmd/bootnode

cluster: build
	scripts/boot-cluster.sh

test:
	go test -v github.com/ethereum/chain/swarm/...
