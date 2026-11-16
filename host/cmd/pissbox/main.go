package main

import (
	"os"
	"pissbox/host/launcher"
)

func main() { os.Exit(launcher.Main(os.Args[1:])) }
