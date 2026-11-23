package openapi

import (
	"encoding/json"
	"testing"
)

// StripeBillingStatusResponse publishes `status` as its discriminator, and the patched decoder
// dispatches on it. The generated decoder picked a branch by which required properties were
// present and never read `status`, so a contradictory payload was read as the wrong branch.
func TestStripeBillingStatusDispatchesOnStatus(t *testing.T) {
	var response StripeBillingStatusResponse

	// Says disabled, carries only the enabled branch's required property: rejected.
	if err := json.Unmarshal([]byte(`{"status":"disabled","billing_method":"stripe","portal_url":"https://x"}`), &response); err == nil {
		t.Fatalf("accepted a contradictory discriminator: %#v", response.GetActualInstance())
	}
	// Neither branch's required property is present: rejected.
	if err := json.Unmarshal([]byte(`{"status":"enabled","billing_method":"stripe"}`), &response); err == nil {
		t.Fatalf("accepted an incomplete enabled branch: %#v", response.GetActualInstance())
	}
	// The status decides; a property of the other branch is an ignored unknown property.
	if err := json.Unmarshal([]byte(`{"status":"enabled","billing_method":"stripe","portal_url":"https://x","reason":"nope"}`), &response); err != nil {
		t.Fatal(err)
	}
	assertSingleBranch(t, response, "StripeBillingEnabled")
	// A status added by a newer server is kept as sent.
	future := `{"status":"suspended","billing_method":"stripe","until":"2027-01-01"}`
	if err := json.Unmarshal([]byte(future), &response); err != nil {
		t.Fatal(err)
	}
	assertSingleBranch(t, response, "Raw")
	if string(response.Raw) != future {
		t.Fatalf("Raw = %s", response.Raw)
	}
}
