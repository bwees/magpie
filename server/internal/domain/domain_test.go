package domain

import (
	"context"
	"slices"
	"testing"
)

func TestNewIDIsValidAndSortsInCreationOrder(t *testing.T) {
	ids := make([]TitleID, 0, 100)
	for range cap(ids) {
		ids = append(ids, NewID[TitleID]())
	}

	for _, id := range ids {
		if !ValidID(id) {
			t.Fatalf("NewID produced invalid ULID %q", id)
		}
	}
	if !slices.IsSorted(ids) {
		t.Error("IDs from one process are not in creation order")
	}
}

func TestValidIDRejectsGarbage(t *testing.T) {
	for _, id := range []UserID{"", "not-a-ulid", "01ARZ3NDEKTSV4RRFFQ69G5FA"} {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true", id)
		}
	}
}

func TestPrincipalRoundTrip(t *testing.T) {
	want := Principal{User: NewID[UserID](), Admin: true}

	got, ok := PrincipalFrom(WithPrincipal(context.Background(), want))
	if !ok || got != want {
		t.Fatalf("PrincipalFrom = %+v, %v; want %+v", got, ok, want)
	}
}

func TestPrincipalMissing(t *testing.T) {
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Fatal("PrincipalFrom found a principal on an empty context")
	}
}
