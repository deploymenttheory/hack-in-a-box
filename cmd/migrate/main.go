package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/deploymenttheory/hack-in-a-box/internal/app"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
)

func main() { os.Exit(app.Main("migrate", run)) }

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool, 0); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	logger.Info("database migrations complete", "version", database.SchemaVersion)
	return nil
}
