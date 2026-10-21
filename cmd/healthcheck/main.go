// Healthcheck is a minimal container probe; it does not require curl in the image.
package main

import (
	"context"
	"net/http"
	"os"
	"time"
)

func main() { os.Exit(run()) }

func run() int {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://127.0.0.1:8080/readyz",
		nil,
	)
	if err != nil {
		return 1
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 1
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
