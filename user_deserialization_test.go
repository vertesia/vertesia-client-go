package vertesia

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// Shared user responses exercise generated timestamp and account-member decoding.

// Every optional field populated, so each generated field is exercised.
func populatedUserJSON(t *testing.T) string { return studioFixture(t, "user-populated.json") }

// A user Mongo holds with neither an email nor timestamps. The mapper supplies all three, which is
// what makes this response deserializable at all — the pre-migration handler omitted them.
func legacyUserJSON(t *testing.T) string { return studioFixture(t, "user-legacy-normalized.json") }

func studioFixture(t *testing.T, name string) string {
	t.Helper()
	directory := os.Getenv("VERTESIA_STUDIO_OPENAPI_FIXTURES")
	if directory == "" {
		t.Skip("Studio fixtures are supplied by SDK sync")
	}
	body, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatalf("cannot read fixture %s: %v", name, err)
	}
	return string(body)
}

func TestDeserializesAFullyPopulatedUser(t *testing.T) {
	var user openapi.User
	if err := json.Unmarshal([]byte(populatedUserJSON(t)), &user); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if user.Id != "68b1779130afe5403a1589ba" {
		t.Fatalf("id = %q", user.Id)
	}
	if user.Email != "someone@acme.com" {
		t.Fatalf("email = %q", user.Email)
	}
	if user.Phone == nil || *user.Phone != "+33100000000" {
		t.Fatalf("phone = %v", user.Phone)
	}
	if len(user.Compartments) != 1 || user.Compartments[0] != "eu" {
		t.Fatalf("compartments = %v", user.Compartments)
	}
}

// The point of `format: date-time` on the schema, and of the two fields being required: a required
// date-time is a plain time.Time, so a caller reads it without a nil check and without parsing.
func TestMapsTheTimestampsToTime(t *testing.T) {
	var user openapi.User
	if err := json.Unmarshal([]byte(populatedUserJSON(t)), &user); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	want := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	if !user.CreatedAt.Equal(want) {
		t.Fatalf("created_at = %v, want %v", user.CreatedAt, want)
	}
	if !user.UpdatedAt.Equal(time.Date(2026, time.July, 30, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("updated_at = %v", user.UpdatedAt)
	}
	// Optional fields stay pointers, so "absent" and "empty" remain distinguishable. That contrast
	// is the whole reason the timestamps were made required rather than optional-with-a-default.
	if user.Picture == nil {
		t.Fatalf("picture should be set on the populated fixture")
	}
}

func TestDeserializesTheNormalizedLegacyResponse(t *testing.T) {
	// The three normalizations the mapper performs, seen from the client: an empty email instead of
	// an absent one, and timestamps derived from the ObjectId.
	var user openapi.User
	if err := json.Unmarshal([]byte(legacyUserJSON(t)), &user); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if user.Email != "" {
		t.Fatalf("email = %q, want the empty-string normalization", user.Email)
	}
	want := time.Date(2025, time.August, 29, 9, 49, 5, 0, time.UTC)
	if !user.CreatedAt.Equal(want) {
		t.Fatalf("created_at = %v, want %v", user.CreatedAt, want)
	}
	// Absent optionals stay nil rather than becoming zero values the caller cannot tell apart.
	if user.Phone != nil {
		t.Fatalf("phone = %v, want nil", user.Phone)
	}
}

func TestDeserializesTheAccountMembersListing(t *testing.T) {
	// UserArray items $ref User, so the listing is checked member by member. A mixed page — one
	// current document, one normalized legacy document — is the realistic case.
	var users []openapi.User
	body := "[" + populatedUserJSON(t) + "," + legacyUserJSON(t) + "]"
	if err := json.Unmarshal([]byte(body), &users); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len = %d", len(users))
	}
	if users[0].Email != "someone@acme.com" || users[1].Email != "" {
		t.Fatalf("emails = %q, %q", users[0].Email, users[1].Email)
	}
	if users[1].CreatedAt.IsZero() {
		t.Fatalf("the legacy member lost its synthesized created_at")
	}
}

func TestRejectsAResponseMissingARequiredTimestamp(t *testing.T) {
	// A member without created_at is what GET /account/members could emit before it was mapped
	// through toUserResponse. The generated UnmarshalJSON checks required properties, so it is an
	// error rather than a zero-valued time a caller would misread as 1 January year 1.
	missing := `{"id":"68b1779130afe5403a1589bc","email":"x@acme.com","name":"NoStamps"}`
	var user openapi.User
	if err := json.Unmarshal([]byte(missing), &user); err == nil {
		t.Fatalf("expected an error for a response missing created_at, got %+v", user)
	}

	// And it fails the whole listing, not just the member — which is what makes converting the
	// array slot load-bearing rather than cosmetic.
	var users []openapi.User
	body := "[" + populatedUserJSON(t) + "," + missing + "]"
	if err := json.Unmarshal([]byte(body), &users); err == nil {
		t.Fatalf("expected the listing to fail on an invalid member, got %+v", users)
	}
}
