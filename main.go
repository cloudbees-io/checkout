package main

import (
	"log"

	"github.com/cloudbees-io/checkout/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
