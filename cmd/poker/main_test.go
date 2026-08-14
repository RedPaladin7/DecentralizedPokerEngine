package main

import (
	"strings"
	"testing"
)

func TestRequireP2PSeats_RejectsTwo(t *testing.T) {
	err := requireP2PSeats(2)
	if err == nil {
		t.Fatal("expected error for 2 seats")
	}
	if !strings.Contains(err.Error(), "3") {
		t.Fatalf("error %q should mention 3", err.Error())
	}
}

func TestRequireP2PSeats_AcceptsThree(t *testing.T) {
	if err := requireP2PSeats(3); err != nil {
		t.Fatalf("3 seats should be accepted: %v", err)
	}
}
