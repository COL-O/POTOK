.PHONY: check run simple drone clean

check:
	./bootstrap.sh --check

run:
	go run ./cmd/organism/...

simple:
	go run ./examples/simple/main.go

drone:
	go run ./examples/drone/main.go

clean:
	rm -rf bin
