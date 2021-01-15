# Mod-Gearman-Worker

[![Build Status](https://github.com/ConSol/mod-gearman-worker-go/workflows/citest/badge.svg)](https://github.com/ConSol/mod-gearman-worker-go/actions?query=workflow:citest)
[![Go Report Card](https://goreportcard.com/badge/github.com/ConSol/mod-gearman-worker-go)](https://goreportcard.com/report/github.com/ConSol/mod-gearman-worker-go)
[![License: GPL v3](https://img.shields.io/badge/License-GPL%20v3-blue.svg)](http://www.gnu.org/licenses/gpl-3.0)

this is a Mod-Gearman-Worker rewrite in golang. It supports all original
command line parameters and therefor can be replace the c-worker without any
config changes.
Since it uses a go routines model instead of preforking workers, it uses much
less ressources than the original worker.


## Embedded Perl

This worker does not support embedded perl. It will run perl scripts simply
like any other plugin.

## Prometheus

Prometheus metrics will get exported when started with the `prometheus-server` option.

    %> .../mod_gearman_worker --prometheus_server=127.0.0.1:8001


## Build Instructions / Installation

Either use `go install` like:

    %> go install github.com/ConSol/mod-gearman-worker-go/cmd/mod_gearman_worker
    %> go install github.com/ConSol/mod-gearman-worker-go/cmd/send_gearman

Or clone the repository and build it manually:

    %> go get github.com/ConSol/mod-gearman-worker-go
    %> cd $GOPATH/src/ConSol/mod-gearman-worker-go
    %> make

### Windows Builds

Windows builds, for example a `send_gearman.exe` can be created by cloning the
repository and running:

    # 64bit windows builds
    %> make build-windows-amd64

    # 32bit windows builds
    %> make build-windows-i386
