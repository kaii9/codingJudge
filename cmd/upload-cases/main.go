package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kaii9/codingJudge/internal/caseassets"
	"github.com/kaii9/codingJudge/internal/objectstore"
	"github.com/kaii9/codingJudge/internal/store"
)

func main() {
	var casesDir string
	var problems string
	var all bool
	flag.StringVar(&casesDir, "cases-dir", "testdata/cases", "root directory containing {problem_id}/{case}.in and {case}.out files")
	flag.StringVar(&problems, "problems", "", "comma-separated problem ids to upload")
	flag.BoolVar(&all, "all", false, "upload every problem directory found under -cases-dir")
	flag.Parse()

	problemIDs, err := selectedProblems(casesDir, problems, all)
	if err != nil {
		slog.Error("select problem cases failed", "error", err)
		os.Exit(2)
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(2)
	}
	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	minioAccessKey := os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("MINIO_SECRET_KEY")
	if minioEndpoint == "" || minioAccessKey == "" || minioSecretKey == "" {
		slog.Error("MINIO_ENDPOINT, MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required")
		os.Exit(2)
	}
	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "codingjudge-assets"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.NewPostgresStore(ctx, databaseURL)
	if err != nil {
		slog.Error("store setup failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	objects, err := objectstore.NewMinIO(objectstore.MinIOConfig{
		Endpoint:  minioEndpoint,
		AccessKey: minioAccessKey,
		SecretKey: minioSecretKey,
		Bucket:    bucket,
		UseSSL:    os.Getenv("MINIO_USE_SSL") == "true",
	})
	if err != nil {
		slog.Error("object store setup failed", "error", err)
		os.Exit(1)
	}
	if err := objects.EnsureBucket(ctx); err != nil {
		slog.Error("bucket setup failed", "bucket", bucket, "error", err)
		os.Exit(1)
	}

	uploader := caseassets.NewUploader(st, objects)
	for _, problemID := range problemIDs {
		report, err := uploader.UploadProblem(ctx, casesDir, problemID)
		if err != nil {
			slog.Error("upload problem cases failed", "problem_id", problemID, "error", err)
			os.Exit(1)
		}
		fmt.Printf("uploaded problem=%s cases=%d\n", report.ProblemID, report.CaseCount)
	}
}

func selectedProblems(casesDir, problems string, all bool) ([]string, error) {
	if all {
		return caseassets.DiscoverProblems(casesDir)
	}
	if strings.TrimSpace(problems) == "" {
		return nil, fmt.Errorf("missing required -problems or -all")
	}
	return splitCSV(problems), nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
