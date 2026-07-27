package judge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/kai/codingjudge/internal/domain"
)

type fakeObjectStore struct {
	objects map[string][]byte
}

func (s fakeObjectStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data := s.objects[key]
	return append([]byte(nil), data...), nil
}

func TestResolveProblemTestCaseAssetsLoadsObjectBackedCase(t *testing.T) {
	t.Parallel()

	input := []byte("1 2\n")
	output := []byte("3\n")
	problem := domain.Problem{
		ID: "sum",
		TestCases: []domain.TestCase{{
			InputObjectKey:          "cases/sum/001.in",
			ExpectedOutputObjectKey: "cases/sum/001.out",
			InputSHA256:             sha256Hex(input),
			ExpectedOutputSHA256:    sha256Hex(output),
			InputSizeBytes:          int64(len(input)),
			ExpectedOutputSizeBytes: int64(len(output)),
		}},
	}
	objects := fakeObjectStore{objects: map[string][]byte{
		"cases/sum/001.in":  input,
		"cases/sum/001.out": output,
	}}

	resolved, err := ResolveProblemTestCaseAssets(context.Background(), problem, objects)
	if err != nil {
		t.Fatalf("ResolveProblemTestCaseAssets returned error: %v", err)
	}
	if resolved.TestCases[0].Input != string(input) {
		t.Fatalf("input = %q, want %q", resolved.TestCases[0].Input, input)
	}
	if resolved.TestCases[0].ExpectedOutput != string(output) {
		t.Fatalf("expected output = %q, want %q", resolved.TestCases[0].ExpectedOutput, output)
	}
}

func TestResolveProblemTestCaseAssetsRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	problem := domain.Problem{
		ID: "sum",
		TestCases: []domain.TestCase{{
			InputObjectKey: "cases/sum/001.in",
			InputSHA256:    sha256Hex([]byte("expected\n")),
		}},
	}
	objects := fakeObjectStore{objects: map[string][]byte{
		"cases/sum/001.in": []byte("tampered\n"),
	}}

	if _, err := ResolveProblemTestCaseAssets(context.Background(), problem, objects); err == nil {
		t.Fatal("ResolveProblemTestCaseAssets should reject checksum mismatch")
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
