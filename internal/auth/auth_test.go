package auth_test

import (
	"testing"

	"github.com/kaii9/codingJudge/internal/auth"
)

func TestPasswordHashDoesNotStorePlaintextAndVerifies(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password hash must not equal plaintext")
	}
	if !auth.CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to verify")
	}
	if auth.CheckPassword(hash, "wrong password") {
		t.Fatal("wrong password should not verify")
	}
}

func TestSessionTokenHashIsStableAndDoesNotExposeToken(t *testing.T) {
	t.Parallel()

	token, err := auth.NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("token must not be empty")
	}
	first := auth.HashSessionToken(token)
	second := auth.HashSessionToken(token)
	if first == "" || first != second {
		t.Fatalf("hash should be stable, got %q and %q", first, second)
	}
	if first == token {
		t.Fatal("stored session hash must not equal raw token")
	}
}
