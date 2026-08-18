package main

import (
	"os"

	"github.com/limecloud/contentcloud/internal/transport/cli"
)

func main() { os.Exit(cli.Execute()) }
