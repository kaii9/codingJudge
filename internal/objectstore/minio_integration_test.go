//go:build integration

package objectstore

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMinIOPutGetRoundTrip(t *testing.T) {
	endpoint := os.Getenv("TEST_MINIO_ENDPOINT")
	accessKey := os.Getenv("TEST_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("TEST_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Skip("TEST_MINIO_ENDPOINT, TEST_MINIO_ACCESS_KEY and TEST_MINIO_SECRET_KEY are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := NewMinIO(MinIOConfig{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Bucket:    "codingjudge-test-assets",
		UseSSL:    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}

	key := "integration/minio-roundtrip.txt"
	want := []byte("object-backed test case\n")
	if err := store.Put(ctx, key, want, "text/plain"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("object content = %q, want %q", got, want)
	}
}
