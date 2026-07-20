package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const usage = `usage: migrate <up|status|down>

  up      apply all pending migrations
  status  show applied and pending migrations
  down    roll back the most recent migration (local dev only)
`

var validCommands = map[string]struct{}{
	"up":     {},
	"status": {},
	"down":   {},
}

func main() {
	logger := config.Log{}.NewLogger(os.Stderr)
	slog.SetDefault(logger)

	cmd, err := parseCommand()
	if err != nil {
		logger.Error("invalid arguments", slog.Any("error", err))
		_, _ = fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	if err := run(logger, cmd); err != nil {
		logger.Error("migrate command failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger, cmd string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pgCfg, err := config.LoadPostgres()
	if err != nil {
		return fmt.Errorf("load postgres config: %w", err)
	}

	db, err := openDB(ctx, pgCfg.DSN())
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn("close postgres failed", slog.Any("error", err))
		}
	}()

	logger.Info("running migrate command", slog.String("cmd", cmd))
	return execCommand(ctx, logger, cmd, db)
}

func parseCommand() (string, error) {
	if len(os.Args) != 2 {
		return "", errors.New("expected exactly one command")
	}
	cmd := os.Args[1]
	if _, ok := validCommands[cmd]; !ok {
		return "", fmt.Errorf("unknown command %q", cmd)
	}
	return cmd, nil
}

func openDB(ctx context.Context, dsn string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}
	db := stdlib.OpenDB(*cfg)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

func execCommand(ctx context.Context, logger *slog.Logger, cmd string, db *sql.DB) error {
	switch cmd {
	case "up":
		if err := migrations.Up(ctx, db); err != nil {
			return err
		}
		logger.Info("migrations applied")
		return nil
	case "status":
		rows, err := migrations.Status(ctx, db)
		if err != nil {
			return err
		}
		return printStatus(os.Stdout, rows)
	case "down":
		if err := migrations.Down(ctx, db); err != nil {
			return err
		}
		logger.Info("migration rolled back")
		return nil
	default:
		return errors.New("unknown command: " + cmd)
	}
}

func printStatus(w io.Writer, rows []*goose.MigrationStatus) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "VERSION\tSTATE\tAPPLIED AT\tSOURCE")
	for _, r := range rows {
		applied := "—"
		if !r.AppliedAt.IsZero() {
			applied = r.AppliedAt.UTC().Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", r.Source.Version, r.State, applied, r.Source.Path)
	}
	return tw.Flush()
}
