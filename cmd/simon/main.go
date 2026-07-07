package main

import (
	"fmt"

	"simon-go/internal/config"
)

func main() {
	cfg := config.Load()
	fmt.Println("simon-go", cfg.DefaultModel)
}
