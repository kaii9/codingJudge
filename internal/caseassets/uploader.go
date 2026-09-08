package caseassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaii9/codingJudge/internal/domain"
)

type ObjectWriter interface {
	Put(context.Context, string, []byte, string) error
}

type Repository interface {
	ReplaceProblemTestCases(context.Context, string, []domain.TestCase) error
}

type Uploader struct {
	repo    Repository
	objects ObjectWriter
}

type UploadReport struct {
	ProblemID string
	CaseCount int
}

func NewUploader(repo Repository, objects ObjectWriter) *Uploader {
	return &Uploader{repo: repo, objects: objects}
}

func DiscoverProblems(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	problems := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		caseEntries, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		for _, caseEntry := range caseEntries {
			if !caseEntry.IsDir() && filepath.Ext(caseEntry.Name()) == ".in" {
				problems = append(problems, entry.Name())
				break
			}
		}
	}
	sort.Strings(problems)
	return problems, nil
}

func (u *Uploader) UploadProblem(ctx context.Context, root, problemID string) (UploadReport, error) {
	if strings.TrimSpace(problemID) == "" {
		return UploadReport{}, fmt.Errorf("problem id is required")
	}
	problemDir := filepath.Join(root, problemID)
	entries, err := os.ReadDir(problemDir)
	if err != nil {
		return UploadReport{}, err
	}
	bases := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".in" {
			continue
		}
		bases = append(bases, strings.TrimSuffix(entry.Name(), ".in"))
	}
	sort.Strings(bases)
	if len(bases) == 0 {
		return UploadReport{}, fmt.Errorf("no .in cases found for problem %q", problemID)
	}

	cases := make([]domain.TestCase, 0, len(bases))
	for _, base := range bases {
		inputPath := filepath.Join(problemDir, base+".in")
		outputPath := filepath.Join(problemDir, base+".out")
		input, err := os.ReadFile(inputPath)
		if err != nil {
			return UploadReport{}, err
		}
		output, err := os.ReadFile(outputPath)
		if err != nil {
			return UploadReport{}, fmt.Errorf("read output pair for %s: %w", inputPath, err)
		}
		inputSHA := sha256Hex(input)
		outputSHA := sha256Hex(output)
		inputKey := fmt.Sprintf("cases/%s/%s/input-%s.txt", problemID, base, inputSHA)
		outputKey := fmt.Sprintf("cases/%s/%s/output-%s.txt", problemID, base, outputSHA)
		if err := u.objects.Put(ctx, inputKey, input, "text/plain; charset=utf-8"); err != nil {
			return UploadReport{}, fmt.Errorf("upload %s: %w", inputKey, err)
		}
		if err := u.objects.Put(ctx, outputKey, output, "text/plain; charset=utf-8"); err != nil {
			return UploadReport{}, fmt.Errorf("upload %s: %w", outputKey, err)
		}
		cases = append(cases, domain.TestCase{
			InputObjectKey:          inputKey,
			ExpectedOutputObjectKey: outputKey,
			InputSHA256:             inputSHA,
			ExpectedOutputSHA256:    outputSHA,
			InputSizeBytes:          int64(len(input)),
			ExpectedOutputSizeBytes: int64(len(output)),
			Hidden:                  true,
		})
	}
	if err := u.repo.ReplaceProblemTestCases(ctx, problemID, cases); err != nil {
		return UploadReport{}, err
	}
	return UploadReport{ProblemID: problemID, CaseCount: len(cases)}, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
