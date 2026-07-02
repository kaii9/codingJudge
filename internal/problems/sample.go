package problems

import "github.com/kai/codingjudge/internal/domain"

func SampleProblems() []domain.Problem {
	return []domain.Problem{
		{
			ID:            "sum",
			Title:         "A+B Problem",
			Description:   "Read two integers from standard input and print their sum.",
			Language:      domain.LanguageGo,
			TimeLimitMS:   2000,
			MemoryLimitMB: 64,
			Difficulty:    domain.DifficultyEasy, Collection: domain.CollectionStarter, SortOrder: 1, Tags: []string{"starter"},
			TestCases: []domain.TestCase{
				{Input: "1 2\n", ExpectedOutput: "3\n"},
				{Input: "10 32\n", ExpectedOutput: "42\n"},
			},
		},
		{
			ID:            "echo",
			Title:         "Echo",
			Description:   "Read one line and print it unchanged.",
			Language:      domain.LanguageGo,
			TimeLimitMS:   2000,
			MemoryLimitMB: 64,
			Difficulty:    domain.DifficultyEasy, Collection: domain.CollectionStarter, SortOrder: 2, Tags: []string{"starter"},
			TestCases: []domain.TestCase{
				{Input: "hello\n", ExpectedOutput: "hello\n"},
			},
		},
	}
}
