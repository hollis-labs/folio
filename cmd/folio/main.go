// Command folio scaffolds new projects from preset manifests.
//
// This is the P0 skeleton entrypoint. Full CLI surface lands in P5.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "folio: v0 skeleton — CLI surface lands in P5")
	os.Exit(1)
}
