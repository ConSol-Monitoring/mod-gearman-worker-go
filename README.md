# Mod-Gearman-Worker

[![Build Status](https://github.com/consol-monitoring/mod-gearman-worker-go/workflows/citest/badge.svg)](https://github.com/consol-monitoring/mod-gearman-worker-go/actions?query=workflow:citest)
[![Go Report Card](https://goreportcard.com/badge/github.com/consol-monitoring/mod-gearman-worker-go)](https://goreportcard.com/report/github.com/consol-monitoring/mod-gearman-worker-go)
[![License: GPL v3](https://img.shields.io/badge/License-GPL%20v3-blue.svg)](http://www.gnu.org/licenses/gpl-3.0)

this is a Mod-Gearman-Worker rewrite in golang. It supports all original
command line parameters and therefor can be replace the c-worker without any
config changes.
Since it uses a go routines model instead of pre-forking workers, it uses much
less resources than the original worker.

## Embedded Perl

This worker does support embedded perl as well. This is done by a (managed) perl
epn daemon which will handle the perl plugins.

## Negate

To make checks more efficient, this worker has implemented its own negate plugin.
Whenever the command line starts with something like .../negate (if the basename
of the first command equals "negate") it will use the internal negate instead of
running the specified negate command.
All options are similar to the
[official negate](https://www.monitoring-plugins.org/doc/man/negate.html) implementation:

## Prometheus

Prometheus metrics will get exported when started with the `prometheus-server` option.

    %> .../mod_gearman_worker --prometheus_server=127.0.0.1:8001

## Build Instructions / Installation

Clone the repository and run the build make target:

    %> git clone http://github.com/consol-monitoring/mod-gearman-worker-go
    %> cd mod-gearman-worker-go
    %> make

### Windows Builds

Windows builds, for example a `send_gearman.exe` can be created by cloning the
repository and running:

    # 64bit windows builds
    %> make build-windows-amd64

    # 32bit windows builds
    %> make build-windows-i386
