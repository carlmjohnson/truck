package main

import (
	"os"

	"github.com/carlmjohnson/exitcode"
	"github.com/carlmjohnson/truck/truckapp"
)

func main() {
	exitcode.Exit(truckapp.CLI(os.Args[1:]))
}
