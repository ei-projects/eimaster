VERSION ?= $(shell git describe --tags)
LDFLAGS = -ldflags "-w -X main.version=${VERSION}"

.PHONY: all build clean

all: build

build:
	@mkdir -p bin
	@go build ${LDFLAGS} -o bin/eimaster ./cmd/master
	@go build ${LDFLAGS} -o bin/lzevil ./cmd/lzevil

clean:
	@rm -rf bin
