package handler

import "testing"

func TestNormalizeMemberRoleAllowsReporter(t *testing.T) {
	role, ok := normalizeMemberRole("reporter")
	if !ok {
		t.Fatal("reporter role must be accepted")
	}
	if role != "reporter" {
		t.Fatalf("role = %q, want reporter", role)
	}
}
