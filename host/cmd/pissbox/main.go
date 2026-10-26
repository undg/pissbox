package main

import (
	"os"
	"pissbox/host/internal/launcher"
)

func main() { os.Exit(launcher.Main(os.Args[1:])) }
