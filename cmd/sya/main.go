package main

import (
	"fmt"
	"os"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("sya %s\n", version)
			return
		}
	}
	fmt.Println("sya — git-native issue tracker for AI-agent workflows (work in progress)")
	fmt.Println("usage: sya version")
}
