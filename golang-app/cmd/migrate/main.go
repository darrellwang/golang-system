// Package main provides a simple database migration CLI for golang-app.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const usage = `Usage: migrate-cli <command> [options]

Commands:
  up                  Run all available migrations
  down [n]            Roll back the last (or n) migration(s)
  version             Show current migration version
  force version       Force migration version (use with caution)
  create name [ext]   Create a new migration file

Environment Variables:
  DATABASE_URL        Database connection string (DSN)
  MIGRATE_PATH        Path to migrations directory (default: ./db/migrations)
`

var (
	dbURL       = os.Getenv("DATABASE_URL")
	migratePath = os.Getenv("MIGRATE_PATH")
)

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	if migratePath == "" {
		migratePath = "./db/migrations"
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutdown signal received")
		os.Exit(1)
	}()

	command := os.Args[1]

	switch command {
	case "up":
		runUp()
	case "down":
		n := 1
		if len(os.Args) >= 3 {
			var err error
			n, err = strconv.Atoi(os.Args[2])
			if err != nil || n < 0 {
				fmt.Fprintf(os.Stderr, "Error: invalid count %q\n", os.Args[2])
				os.Exit(1)
			}
		}
		runDown(n)
	case "version":
		showVersion()
	case "force":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: force requires a version number")
			os.Exit(1)
		}
		version, err := strconv.Atoi(os.Args[2])
		if err != nil || version < 0 {
			fmt.Fprintf(os.Stderr, "Error: invalid version %q\n", os.Args[2])
			os.Exit(1)
		}
		forceVersion(version)
	case "create":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: create requires a migration name")
			os.Exit(1)
		}
		ext := ".sql"
		if len(os.Args) >= 4 {
			ext = os.Args[3]
		}
		createMigration(os.Args[2], ext)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n", command)
		os.Exit(1)
	}
}

func getDB() *sql.DB {
	validateDBURL()
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "Error pinging database: %v\n", err)
		os.Exit(1)
	}
	return db
}

func ensureMigrationsTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version   BIGINT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	_, err := db.Exec(query)
	return err
}

func getCurrentVersion(db *sql.DB) (int64, error) {
	var version int64
	query := "SELECT COALESCE(MAX(version), 0) FROM schema_migrations"
	err := db.QueryRow(query).Scan(&version)
	return version, err
}

func getAppliedMigrations(db *sql.DB) ([]struct {
	version  int64
	filename string
}, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []struct {
		version  int64
		filename string
	}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		migrations = append(migrations, struct {
			version  int64
			filename string
		}{version: v})
	}
	return migrations, nil
}

func getMigrationFiles() ([]struct {
	version  int64
	filename string
}, error) {
	entries, err := os.ReadDir(migratePath)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var files []struct {
		version  int64
		filename string
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			version, err := parseMigrationVersion(entry.Name())
			if err != nil {
				continue // Skip files with invalid versions
			}
			files = append(files, struct {
				version  int64
				filename string
			}{
				version:  version,
				filename: entry.Name(),
			})
		}
	}
	return files, nil
}

func parseMigrationVersion(filename string) (int64, error) {
	base := filepath.Base(filename)
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename: %s", base)
	}
	return strconv.ParseInt(parts[0], 10, 64)
}

func validateDBURL() {
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "Error: DATABASE_URL environment variable is required")
		os.Exit(1)
	}
}

func runUp() {
	db := getDB()
	defer db.Close()

	if err := ensureMigrationsTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating migrations table: %v\n", err)
		os.Exit(1)
	}

	currentVersion, _ := getCurrentVersion(db)
	files, err := getMigrationFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading migration files: %v\n", err)
		os.Exit(1)
	}

	// Sort migrations by version to ensure correct order
	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})

	applied := 0
	for _, f := range files {
		if f.version <= currentVersion {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migratePath, f.filename))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading migration %s: %v\n", f.filename, err)
			os.Exit(1)
		}

		// Extract only the Up section to avoid executing Down statements
		upSQL := extractUpMigration(string(content))
		if upSQL == "" {
			fmt.Fprintf(os.Stderr, "Warning: no Up section found in %s, skipping\n", f.filename)
			continue
		}

		sqlStatements := splitSQLStatements(upSQL)
		tx, err := db.Begin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting transaction for %s: %v\n", f.filename, err)
			os.Exit(1)
		}

		for _, stmt := range sqlStatements {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				fmt.Fprintf(os.Stderr, "Error executing migration %s: %v\nStatement: %s\n", f.filename, err, stmt)
				os.Exit(1)
			}
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", f.version); err != nil {
			_ = tx.Rollback()
			fmt.Fprintf(os.Stderr, "Error recording migration %s: %v\n", f.filename, err)
			os.Exit(1)
		}

		if err := tx.Commit(); err != nil {
			fmt.Fprintf(os.Stderr, "Error committing migration %s: %v\n", f.filename, err)
			os.Exit(1)
		}

		fmt.Printf("Applied: %s\n", f.filename)
		applied++
	}

	if applied == 0 {
		fmt.Println("No new migrations to apply. Already up to date.")
	} else {
		fmt.Printf("\n%d migration(s) applied.\n", applied)
	}
}

func runDown(n int) {
	db := getDB()
	defer db.Close()

	if err := ensureMigrationsTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating migrations table: %v\n", err)
		os.Exit(1)
	}

	applied, err := getAppliedMigrations(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading applied migrations: %v\n", err)
		os.Exit(1)
	}

	if len(applied) == 0 {
		fmt.Println("No migrations to roll back.")
		return
	}

	rolledBack := 0
	for i := 0; i < n && i < len(applied); i++ {
		migration := applied[i]
		files, _ := getMigrationFiles()

		var content []byte
		for _, f := range files {
			if f.version == migration.version {
				content, _ = os.ReadFile(filepath.Join(migratePath, f.filename))
				break
			}
		}

		if len(content) == 0 {
			fmt.Fprintf(os.Stderr, "Warning: migration file not found for version %d\n", migration.version)
			continue
		}

		downSQL := extractDownMigration(string(content))
		if downSQL == "" {
			fmt.Fprintf(os.Stderr, "Warning: no Down migration found in version %d\n", migration.version)
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting transaction for rollback %d: %v\n", migration.version, err)
			os.Exit(1)
		}

		if _, err := tx.Exec(downSQL); err != nil {
			_ = tx.Rollback()
			fmt.Fprintf(os.Stderr, "Error rolling back migration %d: %v\n", migration.version, err)
			os.Exit(1)
		}

		if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = $1", migration.version); err != nil {
			_ = tx.Rollback()
			fmt.Fprintf(os.Stderr, "Error removing migration record %d: %v\n", migration.version, err)
			os.Exit(1)
		}

		if err := tx.Commit(); err != nil {
			fmt.Fprintf(os.Stderr, "Error committing rollback for migration %d: %v\n", migration.version, err)
			os.Exit(1)
		}

		fmt.Printf("Rolled back: version %d\n", migration.version)
		rolledBack++
	}

	if rolledBack == 0 {
		fmt.Println("No migrations were rolled back.")
	} else {
		fmt.Printf("\n%d migration(s) rolled back.\n", rolledBack)
	}
}

func showVersion() {
	db := getDB()
	defer db.Close()

	if err := ensureMigrationsTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating migrations table: %v\n", err)
		os.Exit(1)
	}

	version, _ := getCurrentVersion(db)
	files, _ := getMigrationFiles()

	appliedCount := 0
	rows, _ := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	for rows.Next() {
		var v int64
		_ = rows.Scan(&v)
		appliedCount++
	}
	rows.Close()

	fmt.Printf("Current version: %d\n", version)
	fmt.Printf("Migrations on disk: %d\n", len(files))
	fmt.Printf("Applied migrations: %d\n", appliedCount)
}

func forceVersion(version int) {
	db := getDB()
	defer db.Close()

	if err := ensureMigrationsTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating migrations table: %v\n", err)
		os.Exit(1)
	}

	tx, err := db.Begin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting transaction: %v\n", err)
		os.Exit(1)
	}

	if _, err := tx.Exec("DELETE FROM schema_migrations"); err != nil {
		_ = tx.Rollback()
		fmt.Fprintf(os.Stderr, "Error clearing migrations table: %v\n", err)
		os.Exit(1)
	}

	if version > 0 {
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback()
			fmt.Fprintf(os.Stderr, "Error setting version: %v\n", err)
			os.Exit(1)
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Fprintf(os.Stderr, "Error committing force version: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Force set migration version to: %d\n", version)
}

func createMigration(name string, ext string) {
	if err := os.MkdirAll(migratePath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating migrations directory: %v\n", err)
		os.Exit(1)
	}

	timestamp := time.Now().UTC().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s%s", timestamp, name, ext)
	fullPath := filepath.Join(migratePath, filename)

	var content string
	if ext == ".sql" {
		content = `-- +migrate Up
-- Create your SQL here

-- +migrate Down
-- Drop your SQL here
`
	} else {
		content = fmt.Sprintf("-- Migration: %s\n", name)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating migration file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created migration file: %s\n", filename)
}

func extractDownMigration(content string) string {
	parts := strings.SplitN(content, "-- +migrate Down", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func extractUpMigration(content string) string {
	// Extract content between "-- +migrate Up" and "-- +migrate Down"
	upStart := strings.Index(content, "-- +migrate Up")
	if upStart == -1 {
		return ""
	}
	upContent := content[upStart+len("-- +migrate Up"):]

	downStart := strings.Index(upContent, "-- +migrate Down")
	if downStart == -1 {
		return upContent
	}
	return upContent[:downStart]
}

func splitSQLStatements(sql string) []string {
	var statements []string
	var current strings.Builder

	lines := strings.Split(sql, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		current.WriteString(line)
		current.WriteString("\n")
		if strings.Contains(line, ";") {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" && !strings.HasPrefix(stmt, "--") {
				statements = append(statements, stmt)
			}
			current.Reset()
		}
	}

	lastStmt := strings.TrimSpace(current.String())
	if lastStmt != "" && !strings.HasPrefix(lastStmt, "--") {
		statements = append(statements, lastStmt)
	}

	return statements
}
