package judgerpc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	pb "github.com/kaii9/codingJudge/api/gen/judge/v1"
	"github.com/kaii9/codingJudge/internal/domain"
	"time"
)

func Fingerprint(sub domain.Submission) string {
	data, _ := json.Marshal(struct {
		ProblemID string          `json:"problemId"`
		Language  domain.Language `json:"language"`
		Code      string          `json:"code"`
	}{sub.ProblemID, sub.Language, sub.Code})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func encodeSub(s domain.Submission) *pb.SubmissionRecord {
	out := &pb.SubmissionRecord{Id: s.ID, UserId: s.UserID, ProblemId: s.ProblemID, Language: string(s.Language), Code: s.Code, Status: string(s.Status), CreatedAtUnixNano: s.CreatedAt.UnixNano(), UpdatedAtUnixNano: s.UpdatedAt.UnixNano()}
	if s.Result != nil {
		r := s.Result
		out.Result = &pb.Verdict{Status: string(r.Status), Stdout: r.Stdout, Stderr: r.Stderr, ExitCode: int32(r.ExitCode), DurationMs: r.Duration}
	}
	return out
}
func decodeSub(s *pb.SubmissionRecord) domain.Submission {
	if s == nil {
		return domain.Submission{}
	}
	out := domain.Submission{ID: s.Id, UserID: s.UserId, ProblemID: s.ProblemId, Language: domain.Language(s.Language), Code: s.Code, Status: domain.SubmissionStatus(s.Status), CreatedAt: time.Unix(0, s.CreatedAtUnixNano).UTC(), UpdatedAt: time.Unix(0, s.UpdatedAtUnixNano).UTC()}
	if s.Result != nil {
		r := s.Result
		out.Result = &domain.JudgeResult{Status: domain.SubmissionStatus(r.Status), Stdout: r.Stdout, Stderr: r.Stderr, ExitCode: int(r.ExitCode), Duration: r.DurationMs}
	}
	return out
}
func encodeProblem(p domain.Problem) *pb.ProblemSnapshot {
	out := &pb.ProblemSnapshot{ProblemId: p.ID, TimeLimitMs: int32(p.TimeLimitMS), MemoryLimitMb: int32(p.MemoryLimitMB)}
	for _, tc := range p.TestCases {
		out.Cases = append(out.Cases, &pb.TestCase{Id: tc.ID, Input: tc.Input, ExpectedOutput: tc.ExpectedOutput, InputObjectKey: tc.InputObjectKey, OutputObjectKey: tc.ExpectedOutputObjectKey, InputSha256: tc.InputSHA256, OutputSha256: tc.ExpectedOutputSHA256, InputSize: tc.InputSizeBytes, OutputSize: tc.ExpectedOutputSizeBytes, Hidden: tc.Hidden})
	}
	return out
}
func decodeProblem(p *pb.ProblemSnapshot) domain.Problem {
	if p == nil {
		return domain.Problem{}
	}
	out := domain.Problem{ID: p.ProblemId, TimeLimitMS: int(p.TimeLimitMs), MemoryLimitMB: int(p.MemoryLimitMb)}
	for _, tc := range p.Cases {
		out.TestCases = append(out.TestCases, domain.TestCase{ID: tc.Id, Input: tc.Input, ExpectedOutput: tc.ExpectedOutput, InputObjectKey: tc.InputObjectKey, ExpectedOutputObjectKey: tc.OutputObjectKey, InputSHA256: tc.InputSha256, ExpectedOutputSHA256: tc.OutputSha256, InputSizeBytes: tc.InputSize, ExpectedOutputSizeBytes: tc.OutputSize, Hidden: tc.Hidden})
	}
	return out
}
