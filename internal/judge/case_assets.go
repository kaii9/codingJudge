package judge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/kaii9/codingJudge/internal/domain"
)

type ObjectGetter interface {
	Get(context.Context, string) ([]byte, error)
}

func ResolveProblemTestCaseAssets(ctx context.Context, problem domain.Problem, objects ObjectGetter) (domain.Problem, error) {
	if objects == nil {
		for _, tc := range problem.TestCases {
			if tc.InputObjectKey != "" || tc.ExpectedOutputObjectKey != "" {
				return domain.Problem{}, fmt.Errorf("problem %q has object-backed test cases but no object store is configured", problem.ID)
			}
		}
		return problem, nil
	}
	resolved := problem
	resolved.TestCases = append([]domain.TestCase(nil), problem.TestCases...)
	for i := range resolved.TestCases {
		tc := &resolved.TestCases[i]
		if tc.InputObjectKey != "" {
			data, err := objects.Get(ctx, tc.InputObjectKey)
			if err != nil {
				return domain.Problem{}, fmt.Errorf("get input object %q: %w", tc.InputObjectKey, err)
			}
			if err := verifyObjectAsset("input", tc.InputObjectKey, data, tc.InputSHA256, tc.InputSizeBytes); err != nil {
				return domain.Problem{}, err
			}
			tc.Input = string(data)
		}
		if tc.ExpectedOutputObjectKey != "" {
			data, err := objects.Get(ctx, tc.ExpectedOutputObjectKey)
			if err != nil {
				return domain.Problem{}, fmt.Errorf("get expected output object %q: %w", tc.ExpectedOutputObjectKey, err)
			}
			if err := verifyObjectAsset("expected output", tc.ExpectedOutputObjectKey, data, tc.ExpectedOutputSHA256, tc.ExpectedOutputSizeBytes); err != nil {
				return domain.Problem{}, err
			}
			tc.ExpectedOutput = string(data)
		}
	}
	return resolved, nil
}

func verifyObjectAsset(label, key string, data []byte, wantSHA string, wantSize int64) error {
	if wantSize > 0 && int64(len(data)) != wantSize {
		return fmt.Errorf("%s object %q size mismatch: got %d want %d", label, key, len(data), wantSize)
	}
	if wantSHA == "" {
		return nil
	}
	sum := sha256.Sum256(data)
	gotSHA := hex.EncodeToString(sum[:])
	if gotSHA != wantSHA {
		return fmt.Errorf("%s object %q sha256 mismatch: got %s want %s", label, key, gotSHA, wantSHA)
	}
	return nil
}
