package main

import (
	modgearman "github.com/consol-monitoring/mod-gearman-worker-go"
)

// Build contains the current git commit id
// compile passing -ldflags "-X main.Build <build sha1>" to set the id.
var Build string

func main() {
	modgearman.Sendgearman(Build)
}
