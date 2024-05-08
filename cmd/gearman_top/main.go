package main

import (
	"pkg/modgearman"
)

// Build contains the current git commit id
// compile passing -ldflags "-X main.Build <build sha1>" to set the id.
var Build string

func main() {
	// Demo code. Do not use
	//modgearman.Worker(Build)

	//modgearman.Send2gearmandadmin("status\nversion\n", "127.0.0.1", 4730)
	modgearman.GetGearmanServerData("127.0.0.1", 4730)
}
