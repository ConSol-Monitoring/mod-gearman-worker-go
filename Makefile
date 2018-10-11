#!/usr/bin/make -f

MAKE:=make
SHELL:=bash
GOVERSION:=$(shell \
    go version | \
    awk -F'go| ' '{ split($$5, a, /\./); printf ("%04d%04d", a[1], a[2]); exit; }' \
)
MINGOVERSION:=00010007
MINGOVERSIONSTR:=1.7

EXTERNAL_DEPS = \
	github.com/appscode/g2 \
	github.com/appscode/g2/worker \
	github.com/appscode/g2/client \
	github.com/kdar/factorlog \
	github.com/sevlyar/go-daemon \
	github.com/prometheus/client_golang/prometheus \
	github.com/prometheus/client_golang/prometheus/promhttp \
	github.com/davecgh/go-spew/spew \
	golang.org/x/tools/cmd/goimports \
	github.com/golang/lint/golint \
	github.com/fzipp/gocyclo \
	github.com/client9/misspell/cmd/misspell \
	github.com/jmhodges/copyfighter \
	honnef.co/go/tools/cmd/gosimple \


all: deps fmt build

deps: versioncheck dump
	set -e; for DEP in $(EXTERNAL_DEPS); do \
		go get $$DEP; \
	done

updatedeps: versioncheck
	set -e; for DEP in $(EXTERNAL_DEPS); do \
		go get -u $$DEP; \
	done

dump:
	if [ $(shell grep -rc Dump *.go | grep -v :0 | grep -v dump.go | wc -l) -ne 0 ]; then \
		sed -i.bak 's/\/\/ +build.*/\/\/ build with debug functions/' dump.go; \
	else \
		sed -i.bak 's/\/\/ build.*/\/\/ +build ignore/' dump.go; \
	fi
	rm -f dump.go.bak

build: dump
	go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)"

build-linux-amd64: dump
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -o mod-gearman-worker-go.linux.amd64

debugbuild: deps fmt
	go build -race -ldflags "-X main.Build=$(shell git rev-parse --short HEAD)"

test: fmt dump
	go test -short -v
	if grep -rn TODO: *.go; then exit 1; fi
	if grep -rn Dump *.go | grep -v dump.go; then exit 1; fi

longtest: fmt dump
	go test -v

citest: deps
	#
	# Checking gofmt errors
	#
	if [ $$(gofmt -s -l . | wc -l) -gt 0 ]; then \
		echo "found format errors in these files:"; \
		gofmt -s -l .; \
		exit 1; \
	fi
	#
	# Checking TODO items
	#
	if grep -rn TODO: *.go; then exit 1; fi
	#
	# Checking remaining debug calls
	#
	if grep -rn Dump *.go | grep -v dump.go; then exit 1; fi
	#
	# Run other subtests
	#
	$(MAKE) lint
	$(MAKE) cyclo
	$(MAKE) misspell
	$(MAKE) copyfighter
	$(MAKE) gosimple
	$(MAKE) fmt
	#
	# Normal test cases
	#
	go test -v
	#
	# Benchmark tests
	#
	go test -v -bench=B\* -run=^$$ . -benchmem
	#
	# Race rondition tests
	#
	$(MAKE) racetest
	#
	# All CI tests successful
	#

benchmark: fmt
	go test -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -v -bench=B\* -run=^$$ . -benchmem

racetest: fmt
	go test -race -v

covertest: fmt
	go test -v -coverprofile=cover.out
	go tool cover -func=cover.out
	go tool cover -html=cover.out -o coverage.html

coverweb: fmt
	go test -v -coverprofile=cover.out
	go tool cover -html=cover.out

clean:
	rm -f mod-gearman-worker-go
	rm -f cover.out
	rm -f coverage.html

fmt:
	goimports -w .
	go tool vet -all -shadow -assign -atomic -bool -composites -copylocks -nilfunc -rangeloops -unsafeptr -unreachable .
	gofmt -w -s .

versioncheck:
	@[ $$( printf '%s\n' $(GOVERSION) $(MINGOVERSION) | sort | head -n 1 ) = $(MINGOVERSION) ] || { \
		echo "**** ERROR:"; \
		echo "**** Mod-Gearman-Worker-Go requires at least golang version $(MINGOVERSIONSTR) or higher"; \
		echo "**** this is: $$(go version)"; \
		exit 1; \
	}

lint:
	#
	# Check if golint complains
	# see https://github.com/golang/lint/ for details.
	golint -set_exit_status .

cyclo:
	#
	# Check if there are any too complicated functions
	# Any function with a score higher than 15 is bad.
	# See https://github.com/fzipp/gocyclo for details.
	#
	gocyclo -over 15 . | ./t/filter_cyclo_exceptions.sh

misspell:
	#
	# Check if there are common spell errors.
	# See https://github.com/client9/misspell
	#
	misspell -error .

copyfighter:
	#
	# Check if there are values better passed as pointer
	# See https://github.com/jmhodges/copyfighter
	#
	copyfighter .

gosimple:
	#
	# Check if something could be made simpler
	# See https://github.com/dominikh/go-tools/tree/master/cmd/gosimple
	#
	gosimple

version:
	OLDVERSION="$(shell grep "VERSION =" ./main.go | awk '{print $$3}' | tr -d '"')"; \
	NEWVERSION=$$(dialog --stdout --inputbox "New Version:" 0 0 "v$$OLDVERSION") && \
		NEWVERSION=$$(echo $$NEWVERSION | sed "s/^v//g"); \
		if [ "v$$OLDVERSION" = "v$$NEWVERSION" -o "x$$NEWVERSION" = "x" ]; then echo "no changes"; exit 1; fi; \
		sed -i -e 's/VERSION =.*/VERSION = "'$$NEWVERSION'"/g' ./main.go
