package main

import (
	"os"

	"github.com/deploymenttheory/hack-in-a-box/internal/app"
)

func main() {
	os.Exit(app.Main("worker", app.RunWorker))
}
