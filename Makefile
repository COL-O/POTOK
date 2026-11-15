MODULE := organism
BINARY := bin/organism
IMAGE  := organism:latest

.PHONY: all tidy test race bench-zero-gc bench run simple drone build docker deploy down logs ps monitoring-up monitoring-down clean

all: tidy test race bench-zero-gc build

tidy:
	go mod tidy

test:
	go test -v ./...

race:
	go test -v -race ./...

bench-zero-gc:
	go test -v -run=TestZero ./modules/bus/... ./modules/front/...

bench:
	go test -bench=. -benchmem ./modules/bus/... ./modules/front/...

run:
	go run ./cmd/organism/...

simple:
	go run ./examples/simple/main.go

drone:
	go run ./examples/drone/main.go

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) ./cmd/organism/...

docker:
	docker build -t $(IMAGE) .

deploy:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

monitoring-up:
	docker compose -f deploy/docker-compose.monitoring.yml up -d

monitoring-down:
	docker compose -f deploy/docker-compose.monitoring.yml down

clean:
	rm -rf bin
