package main

import (
	"fmt"
	"os"

	"github.com/mvanhorn/printing-press-library/library/marketing/meta-ads/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
