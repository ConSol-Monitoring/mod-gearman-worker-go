#!/usr/bin/make -f

MAKE:=make
SHELL:=bash
GOVERSION:=$(shell \
    go version | \
    awk -F'go| ' '{ split($$5, a, /\./); printf ("%04d%04d", a[1], a[2]); exit; }' \
)
MINGOVERSION:=00010021
MINGOVERSIONSTR:=1.21
BUILD:=$(shell git rev-parse --short HEAD)
# see https://github.com/go-modules-by-example/index/blob/master/010_tools/README.md
# and https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
TOOLSFOLDER=$(shell pwd)/tools
export GOBIN := $(TOOLSFOLDER)
export PATH := $(GOBIN):$(PATH)
GO=go

all: build

CMDS = $(shell cd ./cmd && ls -1)

tools: | versioncheck
	set -e; for DEP in $(shell grep "_ " buildtools/tools.go | awk '{ print $$2 }' | grep -v go-spew); do \
		( cd buildtools && $(GO) install $$DEP@latest ) ; \
	done
	set -e; for DEP in $(shell grep "_ " buildtools/tools.go | awk '{ print $$2 }' | grep go-spew); do \
		( cd buildtools && $(GO) install $$DEP ) ; \
	done
	( cd buildtools && $(GO) mod tidy )

updatedeps: versioncheck
	$(MAKE) clean
	$(MAKE) tools
	$(GO) mod download
	set -e; for DEP in $(shell grep "_ " buildtools/tools.go | awk '{ print $$2 }'); do \
		( cd buildtools && $(GO) get $$DEP ) ; \
	done
	( cd buildtools && $(GO) get -u )
	$(GO) mod download
	GOPROXY=direct $(GO) get -u
	GOPROXY=direct $(GO) get -t -u
	$(MAKE) cleandeps

cleandeps:
	$(GO) mod tidy
	( cd buildtools && $(GO) mod tidy )

vendor: go.work
	$(GO) mod download
	$(GO) mod tidy
	GOWORK=off $(GO) mod vendor

go.work:
	echo "go $(MINGOVERSIONSTR)" > go.work
	$(GO) work use . buildtools/.

dump:
	if [ $(shell grep -r Dump *.go ./cmd/*/*.go | grep -v Data::Dumper | grep -v dump.go | wc -l) -ne 0 ]; then \
		sed -i.bak -e 's/\/\/go:build.*/\/\/ :build with debug functions/' -e 's/\/\/ +build.*/\/\/ build with debug functions/' dump.go; \
	else \
		sed -i.bak -e 's/\/\/ :build.*/\/\/go:build ignore/' -e 's/\/\/ build.*/\/\/ +build ignore/' dump.go; \
	fi
	rm -f dump.go.bak

build: vendor
	set -e; for CMD in $(CMDS); do \
		( cd ./cmd/$$CMD && go build -ldflags "-s -w -X main.Build=$(BUILD)" -o ../../$$CMD ) ; \
	done

# run build watch, ex. with tracing: make build-watch -- -debug=3 --logfile=stderr
build-watch: vendor
	set -x ; ls *.go cmd/*/*.go worker.cfg | entr -sr "$(MAKE) build && ./mod_gearman_worker $(filter-out $@,$(MAKECMDGOALS)) $(shell echo $(filter-out --,$(MAKEFLAGS)) | tac -s " ")"

build-linux-amd64: vendor
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.Build=$(BUILD)" -o ../../$$CMD.linux.amd64; cd ../..; \
	done

build-windows-i386: vendor
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -ldflags "-s -w -X main.Build=$(BUILD)" -o ../../$$CMD.windows.i386.exe; cd ../..; \
	done

build-windows-amd64: vendor
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.Build=$(BUILD)" -o ../../$$CMD.windows.amd64.exe; cd ../..; \
	done

send_gearman: *.go cmd/send_gearman/*.go
	cd ./cmd/send_gearman && go build -ldflags "-s -w -X main.Build=$(BUILD)" -o ../../send_gearman

send_gearman.exe: *.go cmd/send_gearman/*.go
	cd ./cmd/send_gearman && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.Build=$(BUILD)" -o ../../send_gearman.exe

debugbuild: fmt dump vendor
	go build -race -ldflags "-X main.Build=$(BUILD)"
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && go build -race -ldflags "-X main.Build=$(BUILD)"; cd ../..; \
	done

devbuild: debugbuild

test: dump vendor
	go test -short -v -timeout=1m
	if grep -rn TODO: *.go ./cmd/; then exit 1; fi
	if grep -rn Dump *.go ./cmd/*/*.go | grep -v Data::Dumper | grep -v dump.go; then exit 1; fi

# test with filter
testf: vendor
	go test -short -v -run "$(filter-out $@,$(MAKECMDGOALS))"

longtest: fmt dump vendor
	go test -v -timeout=1m

citest: vendor
	#
	# Checking gofmt errors
	#
	if [ $$(gofmt -s -l *.go ./cmd/ | wc -l) -gt 0 ]; then \
		echo "found format errors in these files:"; \
		gofmt -s -l *.go ./cmd/; \
		exit 1; \
	fi
	#
	# Checking TODO items
	#
	if grep -rn TODO: *.go ./cmd/; then exit 1; fi
	#
	# Checking remaining debug calls
	#
	if grep -rn Dump *.go ./cmd/*/*.go | grep -v Data::Dumper | grep -v dump.go; then exit 1; fi
	#
	# Darwin and Linux should be handled equal
	#
	diff mod_gearman_worker_linux.go mod_gearman_worker_darwin.go
	#
	# Run other subtests
	#
	$(MAKE) golangci
	-$(MAKE) govulncheck
	$(MAKE) fmt
	#
	# Normal test cases
	#
	go test -v -timeout=1m
	#
	# Benchmark tests
	#
	go test -v -timeout=1m -bench=B\* -run=^$$ . -benchmem
	#
	# Race rondition tests
	#
	$(MAKE) racetest
	#
	# Test cross compilation
	#
	$(MAKE) build-linux-amd64
	$(MAKE) build-windows-amd64
	$(MAKE) build-windows-i386
	#
	# All CI tests successful
	#
	go mod tidy

benchmark: fmt
	go test -timeout=1m -ldflags "-s -w -X main.Build=$(BUILD)" -v -bench=B\* -run=^$$ . -benchmem

racetest: fmt
	go test -race -v -timeout=3m -coverprofile=coverage.txt -covermode=atomic

covertest: fmt
	go test -v -coverprofile=cover.out -timeout=1m
	go tool cover -func=cover.out
	go tool cover -html=cover.out -o coverage.html

coverweb: fmt
	go test -v -coverprofile=cover.out -timeout=1m
	go tool cover -html=cover.out

clean:
	set -e; for CMD in $(CMDS); do \
		rm -f ./cmd/$$CMD/$$CMD; \
	done
	rm -f $(CMDS)
	rm -f *.windows.*.exe
	rm -f *.linux.*
	rm -f cover.out
	rm -f coverage.html
	rm -f coverage.txt
	rm -f mod-gearman*.html
	rm -rf vendor/
	rm -rf go.work
	rm -rf go.work.sum
	rm -rf $(TOOLSFOLDER)

GOVET=go vet -all
fmt: tools
	set -e; for CMD in $(CMDS); do \
		$(GOVET) ./cmd/$$CMD; \
	done
	gofmt -w -s *.go ./cmd/*/*.go
	./tools/gofumpt -w *.go ./cmd/*/*.go
	./tools/gci write *.go ./cmd/*/*.go --skip-generated
	goimports -w *.go ./cmd/*/*.go

versioncheck:
	@[ $$( printf '%s\n' $(GOVERSION) $(MINGOVERSION) | sort | head -n 1 ) = $(MINGOVERSION) ] || { \
		echo "**** ERROR:"; \
		echo "**** Mod-Gearman-Worker-Go requires at least golang version $(MINGOVERSIONSTR) or higher"; \
		echo "**** this is: $$(go version)"; \
		exit 1; \
	}

golangci: tools
	#
	# golangci combines a few static code analyzer
	# See https://github.com/golangci/golangci-lint
	#
	golangci-lint run ./...; \

govulncheck: tools
	govulncheck ./...

version:
	OLDVERSION="$(shell grep "VERSION =" ./mod_gearman_worker.go | awk '{print $$3}' | tr -d '"')"; \
	NEWVERSION=$$(dialog --stdout --inputbox "New Version:" 0 0 "v$$OLDVERSION") && \
		NEWVERSION=$$(echo $$NEWVERSION | sed "s/^v//g"); \
		if [ "v$$OLDVERSION" = "v$$NEWVERSION" -o "x$$NEWVERSION" = "x" ]; then echo "no changes"; exit 1; fi; \
		sed -i -e 's/VERSION =.*/VERSION = "'$$NEWVERSION'"/g' *.go cmd/*/*.go

# just skip unknown make targets
.DEFAULT:
	@if [[ "$(MAKECMDGOALS)" =~ ^testf ]]; then \
		: ; \
	else \
		echo "unknown make target(s): $(MAKECMDGOALS)"; \
		exit 1; \
	fi
