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
	github.com/appscode/g2/pkg/runtime \
	github.com/kdar/factorlog \
	github.com/sevlyar/go-daemon \
	github.com/prometheus/client_golang/prometheus \
	github.com/davecgh/go-spew/spew \
	golang.org/x/tools/cmd/goimports \
	golang.org/x/lint/golint \
	github.com/fzipp/gocyclo \
	github.com/client9/misspell/cmd/misspell \
	github.com/jmhodges/copyfighter \
	honnef.co/go/tools/cmd/gosimple \

CMDS = $(shell cd ./cmd && ls -1)

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
	if [ $(shell grep -rc Dump *.go ./cmd/*/*.go | grep -v :0 | grep -v dump.go | wc -l) -ne 0 ]; then \
		sed -i.bak 's/\/\/ +build.*/\/\/ build with debug functions/' dump.go; \
	else \
		sed -i.bak 's/\/\/ build.*/\/\/ +build ignore/' dump.go; \
	fi
	rm -f dump.go.bak

build: dump
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -o ../../$$CMD; cd ../..; \
	done

build-linux-amd64: dump
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -o ../../$$CMD.linux.amd64; cd ../..; \
	done

build-windows-i386: dump
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -o ../../$$CMD.windows.i386.exe; cd ../..; \
	done

build-windows-amd64: dump
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -o ../../$$CMD.windows.amd64.exe; cd ../..; \
	done

send_gearman: *.go cmd/send_gearman/*.go
	cd ./cmd/send_gearman && go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -o ../../send_gearman

send_gearman.exe: *.go cmd/send_gearman/*.go
	cd ./cmd/send_gearman && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.Build=$(shell git rev-parse --short HEAD)" -o ../../send_gearman.exe

debugbuild: deps fmt
	go build -race -ldflags "-X main.Build=$(shell git rev-parse --short HEAD)"
	set -e; for CMD in $(CMDS); do \
		cd ./cmd/$$CMD && go build -race -ldflags "-X main.Build=$(shell git rev-parse --short HEAD)"; cd ../..; \
	done

test: fmt dump
	go test -short -v
	if grep -rn TODO: *.go ./cmd/; then exit 1; fi
	if grep -rn Dump *.go ./cmd/*/*.go | grep -v dump.go; then exit 1; fi

longtest: fmt dump
	go test -v

citest: deps
	#
	# Checking gofmt errors
	#
	if [ $$(gofmt -s -l . ./cmd/ | wc -l) -gt 0 ]; then \
		echo "found format errors in these files:"; \
		gofmt -s -l .; \
		exit 1; \
	fi
	#
	# Checking TODO items
	#
	if grep -rn TODO: *.go ./cmd/; then exit 1; fi
	#
	# Checking remaining debug calls
	#
	if grep -rn Dump *.go ./cmd/*/*.go | grep -v dump.go; then exit 1; fi
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
	# Test cross compilation
	#
	$(MAKE) build-linux-amd64
	$(MAKE) build-windows-amd64
	$(MAKE) build-windows-i386
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
	set -e; for CMD in $(CMDS); do \
		rm -f ./cmd/$$CMD/$$CMD; \
	done
	rm -f $(CMDS)
	rm -f *.windows.*.exe
	rm -f *.linux.*
	rm -f cover.out
	rm -f coverage.html

fmt:
	goimports -w .
	go tool vet -all -shadow -assign -atomic -bool -composites -copylocks -nilfunc -rangeloops -unsafeptr -unreachable *.go
	set -e; for CMD in $(CMDS); do \
		go tool vet -all -shadow -assign -atomic -bool -composites -copylocks -nilfunc -rangeloops -unsafeptr -unreachable ./cmd/$$CMD; \
	done
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
	mv mod_gearman_worker_windows.go mod_gearman_worker_windows.off; \
	mv mod_gearman_worker_darwin.go mod_gearman_worker_darwin.off; \
	copyfighter .; rc=$$?; \
	mv mod_gearman_worker_windows.off mod_gearman_worker_windows.go; \
	mv mod_gearman_worker_darwin.off mod_gearman_worker_darwin.go; \
	exit $$rc

gosimple:
	#
	# Check if something could be made simpler
	# See https://github.com/dominikh/go-tools/tree/master/cmd/gosimple
	#
	gosimple

version:
	OLDVERSION="$(shell grep "VERSION =" ./mod_gearman_worker.go | awk '{print $$3}' | tr -d '"')"; \
	NEWVERSION=$$(dialog --stdout --inputbox "New Version:" 0 0 "v$$OLDVERSION") && \
		NEWVERSION=$$(echo $$NEWVERSION | sed "s/^v//g"); \
		if [ "v$$OLDVERSION" = "v$$NEWVERSION" -o "x$$NEWVERSION" = "x" ]; then echo "no changes"; exit 1; fi; \
		sed -i -e 's/VERSION =.*/VERSION = "'$$NEWVERSION'"/g' *.go cmd/*/*.go
