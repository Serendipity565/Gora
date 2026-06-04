package main

import (
	"fmt"
	"os"

	"github.com/Serendipity565/gora/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
