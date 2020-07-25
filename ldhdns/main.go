package main

import (
	"fmt"
	"os"

	"go.virtualstaticvoid.com/ldhdns/cmd"
)

func main() {
	if err := cmd.Root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
