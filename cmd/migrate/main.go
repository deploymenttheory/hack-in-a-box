package main

import (
	"context"
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
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool, 0); err != nil {
		return err
	}
	logger.Info("database migrations complete", "version", database.SchemaVersion)
	return nil
}
