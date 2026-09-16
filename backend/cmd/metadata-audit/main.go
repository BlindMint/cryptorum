package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"cryptorum/internal/metadataaudit"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = generate(os.Args[2:])
	case "compact":
		err = compact(os.Args[2:])
	case "apply":
		err = apply(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "metadata-audit:", err)
		os.Exit(1)
	}
}

func generate(args []string) error {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	dbPath := flags.String("db", "../data/cryptorum.db", "path to the Cryptorum database")
	output := flags.String("output", "../data/metadata-audit/batch-001", "output directory")
	limit := flags.Int("limit", 100, "maximum proposals in this batch")
	offset := flags.Int("offset", 0, "proposal offset for later batches")
	if err := flags.Parse(args); err != nil {
		return err
	}
	absDB, err := filepath.Abs(*dbPath)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+absDB+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	result, err := metadataaudit.Generate(context.Background(), db, metadataaudit.GenerateOptions{
		SourceDB: absDB, Output: *output, Limit: *limit, Offset: *offset,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Generated %d of %d proposals (%d scoped, %d skipped).\n", result.Selected, result.Candidates, result.Scoped, result.Skipped)
	fmt.Printf("Review: %s\nCompact review: %s\nManifest: %s\nSummary: %s\n", result.ReviewPath, result.CompactPath, result.ManifestPath, result.SummaryPath)
	return nil
}

func compact(args []string) error {
	flags := flag.NewFlagSet("compact", flag.ContinueOnError)
	manifest := flags.String("manifest", "", "path to manifest.json")
	output := flags.String("output", "", "output path; defaults to review-compact.txt beside the manifest")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *manifest == "" {
		return fmt.Errorf("manifest is required")
	}
	if *output == "" {
		*output = filepath.Join(filepath.Dir(*manifest), "review-compact.txt")
	}
	if err := metadataaudit.RenderCompactReview(*manifest, *output); err != nil {
		return err
	}
	fmt.Printf("Compact review: %s\n", *output)
	return nil
}

func apply(args []string) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	dbPath := flags.String("db", "../data/cryptorum.db", "path to the Cryptorum database")
	manifest := flags.String("manifest", "", "path to the immutable manifest.json")
	decisions := flags.String("decisions", "", "path to the reviewed review.csv")
	backupDir := flags.String("backup-dir", "", "backup directory; defaults to data/backups")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *manifest == "" || *decisions == "" {
		return fmt.Errorf("manifest and decisions are required")
	}
	absDB, err := filepath.Abs(*dbPath)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", absDB)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	result, err := metadataaudit.Apply(context.Background(), db, metadataaudit.ApplyOptions{
		SourceDB: absDB, ManifestPath: *manifest, DecisionsPath: *decisions,
		BackupDir: *backupDir, ActorUserID: 1,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Approved: %d; updated: %d; rejected: %d; unreviewed: %d.\n", result.Approved, result.Updated, result.Rejected, result.Unreviewed)
	if result.BackupPath != "" {
		fmt.Printf("Backup: %s\n", result.BackupPath)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: metadata-audit <generate|compact|apply> [options]")
}
