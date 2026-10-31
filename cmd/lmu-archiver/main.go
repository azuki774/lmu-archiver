package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: lmu-archiver [flags]")
		fmt.Fprintln(flag.CommandLine.Output(), "\nA work-in-progress CLI for Le Mans Ultimate.")
		fmt.Fprintln(flag.CommandLine.Output(), "Authentication and archiving are not implemented yet.")
		fmt.Fprintln(flag.CommandLine.Output(), "\nFlags:\n  -h, --help  Show help")
	}
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "Unexpected argument: %s\n\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}
	flag.Usage()
}
