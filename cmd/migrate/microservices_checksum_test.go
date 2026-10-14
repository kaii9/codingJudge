package main

import "testing"

// These checksums were first deployed before the whitespace-only cleanup.
// Applied migrations are immutable bytes, even if a formatter sees no SQL change.
func TestMicroserviceMigrationsRetainFirstAppliedChecksum(t *testing.T) {
	expected := map[string]string{
		"001_judge.sql":             "ab944a7b910e2bb6be12c1a6c27e7c79ad1fb1b2d48a515ad8a88bcb0dcad7bb",
		"002_completion_events.sql": "b424b32eba5a8c65e599abd4296edf866205b2e6df61efa7ee130288e7ed049a",
		"001_platform.sql":          "debeb39189ab68d3567cb6a67f974eb7480b0bbc6c7f8525f471f71ecd907e74",
		"005_projection.sql":        "ace1e0be0c688641d8e1c13933e784d1936fcb4befdadd6e77c87cb79a90553e",
	}
	for _, dir := range []string{"../../migrations/judge", "../../migrations/platform"} {
		items, err := loadMigrations(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if want, found := expected[item.name]; found && item.checksum != want {
				t.Errorf("%s checksum=%s, want first applied %s", item.name, item.checksum, want)
			}
		}
	}
}
