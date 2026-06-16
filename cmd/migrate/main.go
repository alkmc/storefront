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
	"github.com/alkmc/storefront/internal/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const usage = `usage: migrate <command>

commands:
  up      apply all pending migrations
  down    roll back the most recent migration (local dev only)
  status  show applied and pending migrations
`

var validCommands = map[string]struct{}{
	"up":     {},
	"down":   {},
	"status": {},
}

func main() {
	cmd := parseCommand()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := cfg.Log.NewLogger(os.Stderr)
	slog.SetDefault(logger)

	if err := run(logger, cfg, cmd); err != nil {
		logger.Error("migration command failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger, cfg config.Config, cmd string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := openDB(ctx, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn("failed to close db connection", slog.Any("error", err))
		}
	}()

	logger.Info("running migrate command", slog.String("cmd", cmd))
	return dispatch(ctx, logger, cmd, db)
}

func parseCommand() string {
	if len(os.Args) != 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	if _, ok := validCommands[cmd]; !ok {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	return cmd
}

func openDB(ctx context.Context, dsn string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}
	db := stdlib.OpenDB(*cfg)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

func dispatch(ctx context.Context, logger *slog.Logger, cmd string, db *sql.DB) error {
	switch cmd {
	case "up":
		if err := migrate.Up(ctx, db); err != nil {
			return err
		}
		logger.Info("migrations applied")
		return nil
	case "down":
		if err := migrate.Down(ctx, db); err != nil {
			return err
		}
		logger.Info("migration rolled back")
		return nil
	case "status":
		return printStatus(ctx, os.Stdout, db)
	default:
		return errors.New("unknown command: " + cmd)
	}
}

func printStatus(ctx context.Context, w io.Writer, db *sql.DB) error {
	rows, err := migrate.Status(ctx, db)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSION\tSTATE\tAPPLIED AT\tSOURCE")
	for _, r := range rows {
		applied := "—"
		if !r.AppliedAt.IsZero() {
			applied = r.AppliedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", r.Source.Version, r.State, applied, r.Source.Path)
	}
	return tw.Flush()
}
