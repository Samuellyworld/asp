package main

import (
	"fmt"
	"os"
)

var (
	Version   = "0.1.0"
	BuildTime = "development"
	GitCommit = "unknown"
)

func main() {
	fmt.Printf("trading-bot v%s\n", Version)
	fmt.Println("run 'docker-compose up -d postgres redis' to start databases")
	os.Exit(0)
}
