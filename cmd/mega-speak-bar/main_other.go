//go:build !darwin

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "mega-speak-bar: a macOS menu bar app")
	os.Exit(1)
}
