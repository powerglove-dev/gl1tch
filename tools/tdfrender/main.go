// tdfrender renders "GL1TCH" using the embedded inertia TDF font and writes
// ANSI-coloured lines to stdout. Pipe to tools/ansi2html for web output.
package main

import (
	"fmt"
	"os"

	"github.com/8op-org/gl1tch/internal/tdf"
)

func main() {
	f, err := tdf.LoadEmbedded("inertia")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tdfrender: %v\n", err)
		os.Exit(1)
	}
	for _, line := range tdf.RenderString("GL1TCH", f) {
		fmt.Println(line)
	}
}
