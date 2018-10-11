# Mod-Gearman-Worker

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

    %> .../mod-gearman-worker-go --prometheus_server=127.0.0.1:8001
