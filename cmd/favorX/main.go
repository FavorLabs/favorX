package main

import (
	"fmt"
	"os"

	"github.com/FavorLabs/favorX/cmd/favorX/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
