package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockID int64 = 582143901

type migration struct {
	version  int
	name     string
	path     string
	sql      string
	checksum string
}

func main() {
	var dir string
	flag.StringVar(&dir, "dir", "migrations", "directory containing versioned SQL migrations")
	flag.Parse()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(2)
	}
	migrations, err := loadMigrations(dir)
	if err != nil {
		slog.Error("load migrations", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := applyMigrations(ctx, pool, migrations); err != nil {
		slog.Error("apply migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations complete", "count", len(migrations))
}

func loadMigrations(dir string) ([]migration, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no SQL migrations found in %q", dir)
	}
	items := make([]migration, 0, len(paths))
	seen := make(map[int]string)
	for _, path := range paths {
		name := filepath.Base(path)
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("migration %q must use NNN_name.sql format", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migration %q has invalid version", name)
		}
		if previous, exists := seen[version]; exists {
			return nil, fmt.Errorf("migration version %d is duplicated by %q and %q", version, previous, name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		seen[version] = name
		items = append(items, migration{
			version:  version,
			name:     name,
			path:     path,
			sql:      string(data),
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].version < items[j].version })
	return items, nil
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool, migrations []migration) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return err
	}
	var appliedCount int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&appliedCount); err != nil {
		return err
	}
	if appliedCount == 0 {
		if err := baselineLegacySchema(ctx, conn, migrations); err != nil {
			return fmt.Errorf("baseline existing schema: %w", err)
		}
	}

	for _, item := range migrations {
		var checksum string
		err := conn.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, item.version).Scan(&checksum)
		if err == nil {
			if checksum != "baseline" && checksum != item.checksum {
				return fmt.Errorf("migration %03d checksum changed after application", item.version)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, item.sql); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("execute %s: %w", item.name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version,name,checksum) VALUES ($1,$2,$3)`, item.version, item.name, item.checksum); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		slog.Info("applied database migration", "version", item.version, "name", item.name)
	}
	return nil
}

func baselineLegacySchema(ctx context.Context, conn *pgxpool.Conn, migrations []migration) error {
	for _, item := range migrations {
		detected, err := legacyMigrationDetected(ctx, conn, item.version)
		if err != nil {
			return err
		}
		if !detected {
			continue
		}
		if _, err := conn.Exec(ctx, `INSERT INTO schema_migrations (version,name,checksum) VALUES ($1,$2,'baseline') ON CONFLICT (version) DO NOTHING`, item.version, item.name); err != nil {
			return err
		}
	}
	return nil
}

func legacyMigrationDetected(ctx context.Context, conn *pgxpool.Conn, version int) (bool, error) {
	query := ""
	switch version {
	case 1:
		query = `SELECT to_regclass('problems') IS NOT NULL AND to_regclass('problem_test_cases') IS NOT NULL AND to_regclass('submissions') IS NOT NULL`
	case 2:
		var problemsExists bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass('problems') IS NOT NULL`).Scan(&problemsExists); err != nil || !problemsExists {
			return false, err
		}
		query = `SELECT count(*)=2 FROM problems WHERE id IN ('sum','echo')`
	case 3:
		query = `SELECT to_regclass('judge_outbox') IS NOT NULL AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='submissions' AND column_name='judge_token')`
	case 4:
		query = `SELECT to_regclass('problem_tags') IS NOT NULL AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='problems' AND column_name='difficulty')`
	case 5:
		query = `SELECT to_regclass('users') IS NOT NULL AND to_regclass('user_sessions') IS NOT NULL AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='submissions' AND column_name='user_id')`
	case 6:
		query = `SELECT to_regclass('submission_artifacts') IS NOT NULL AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='problem_test_cases' AND column_name='input_object_key')`
	default:
		return false, nil
	}
	var detected bool
	if err := conn.QueryRow(ctx, query).Scan(&detected); err != nil {
		return false, err
	}
	return detected, nil
}
