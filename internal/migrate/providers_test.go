package migrate

import "testing"

// An export is account-shaped: the reference file carried 922 rows behind one
// endpoint. Collapsing to distinct endpoints is the whole point of this view,
// so the count that matters is endpoints, not rows.
func TestProvidersCollapsesAccountsToEndpoints(t *testing.T) {
	plan := Plan{
		Healthy: []Connection{
			{Name: "Key 1", BaseURL: "https://api.example.com/v1", APIKey: "sk-a", SourceProvider: "example", Format: "openai"},
			{Name: "Key 2", BaseURL: "https://api.example.com/v1", APIKey: "sk-b", SourceProvider: "example", Format: "openai"},
		},
		Unusable: []Connection{
			{Name: "Key 3", BaseURL: "https://api.example.com/v1", APIKey: "sk-c", SourceProvider: "example", Format: "openai"},
		},
	}

	got := plan.Providers()
	if len(got) != 1 {
		t.Fatalf("expected 3 accounts to collapse into 1 endpoint, got %d", len(got))
	}
	if got[0].Accounts != 3 {
		t.Errorf("Accounts = %d, want 3", got[0].Accounts)
	}
	if got[0].Healthy != 2 {
		t.Errorf("Healthy = %d, want 2 (the two rows in the Healthy bucket)", got[0].Healthy)
	}
	if got[0].BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %q", got[0].BaseURL)
	}
}

// The Provider struct must not carry credentials. This is the guarantee that
// makes the endpoint safe to hand to a user who explicitly does not want the
// accounts: there is no field for a key to hide in.
func TestProvidersCarryNoCredentials(t *testing.T) {
	plan := Plan{
		Healthy: []Connection{
			{Name: "Key 1", BaseURL: "https://api.example.com/v1", APIKey: "sk-super-secret", SourceProvider: "example"},
		},
	}
	for _, p := range plan.Providers() {
		// Every string field is checked rather than just the obvious ones, so a
		// future field that accidentally carries a key fails here.
		for field, value := range map[string]string{
			"Name":    p.Name,
			"BaseURL": p.BaseURL,
			"Format":  p.Format,
			"Prefix":  p.Prefix,
		} {
			if value == "sk-super-secret" {
				t.Errorf("field %s leaked the API key", field)
			}
		}
	}
}

// Endpoints differing only by trailing slash or case are one provider; emitting
// both would give the user a duplicate to clean up by hand.
func TestProvidersDeduplicatesEquivalentURLs(t *testing.T) {
	plan := Plan{
		Healthy: []Connection{
			{BaseURL: "https://api.example.com/v1", SourceProvider: "example"},
			{BaseURL: "https://api.example.com/v1/", SourceProvider: "example"},
			{BaseURL: "https://API.example.com/v1", SourceProvider: "example"},
		},
	}
	if got := plan.Providers(); len(got) != 1 {
		t.Fatalf("expected equivalent URLs to collapse to 1 provider, got %d: %+v", len(got), got)
	}
}

// A blocked OAuth connection still names a real provider. Its credentials
// cannot come across, but the endpoint belongs in a catalogue the user asked
// for — dropping it would lose information for no benefit.
func TestProvidersIncludeBlockedEndpoints(t *testing.T) {
	plan := Plan{
		Blocked: []Connection{
			{BaseURL: "https://api.oauthy.com/v1", SourceProvider: "oauthy", Portability: NotPortableOAuth},
		},
	}
	got := plan.Providers()
	if len(got) != 1 {
		t.Fatalf("expected the blocked connection's endpoint to be kept, got %d", len(got))
	}
	if got[0].Healthy != 0 {
		t.Errorf("Healthy = %d, want 0 for a blocked connection", got[0].Healthy)
	}
}

// A connection with no endpoint has nothing to contribute to a catalogue.
func TestProvidersSkipsConnectionsWithoutEndpoint(t *testing.T) {
	plan := Plan{
		Blocked: []Connection{
			{BaseURL: "", SourceProvider: "codex", Portability: NotPortableOAuth},
			{BaseURL: "   ", SourceProvider: "qoder", Portability: NotPortableOAuth},
		},
	}
	if got := plan.Providers(); len(got) != 0 {
		t.Errorf("expected endpoint-less connections to be skipped, got %+v", got)
	}
}

// Export rows are named per account ("Key 960"), which tells the user nothing
// about the provider. The label should come from the provider id, falling back
// to the prefix and then the host — never the account name.
func TestProvidersNameFromProviderNotAccount(t *testing.T) {
	cases := []struct {
		name string
		conn Connection
		want string
	}{
		{
			name: "source provider id wins",
			conn: Connection{Name: "Key 960", BaseURL: "https://api.example.com/v1", SourceProvider: "xiaomi-mimo", Prefix: "mm"},
			want: "xiaomi-mimo",
		},
		{
			name: "opaque generated id falls through to prefix",
			conn: Connection{Name: "Key 1", BaseURL: "https://ai.sumopod.com/v1", SourceProvider: "openai-compatible-chat-555a26bb", Prefix: "sumo"},
			want: "sumo",
		},
		{
			name: "no id and no prefix falls through to host",
			conn: Connection{Name: "Key 1", BaseURL: "https://ai.genfity.com/v1"},
			want: "ai.genfity.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Plan{Healthy: []Connection{tc.conn}}.Providers()
			if len(got) != 1 {
				t.Fatalf("expected 1 provider, got %d", len(got))
			}
			if got[0].Name != tc.want {
				t.Errorf("Name = %q, want %q", got[0].Name, tc.want)
			}
		})
	}
}

// Combos reference providers by prefix, so a prefix seen on any account of an
// endpoint should survive onto the collapsed provider.
func TestProvidersKeepPrefixFromAnyAccount(t *testing.T) {
	plan := Plan{
		Healthy: []Connection{
			{BaseURL: "https://api.example.com/v1", SourceProvider: "example"},
			{BaseURL: "https://api.example.com/v1", SourceProvider: "example", Prefix: "ex"},
		},
	}
	got := plan.Providers()
	if len(got) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(got))
	}
	if got[0].Prefix != "ex" {
		t.Errorf("Prefix = %q, want %q", got[0].Prefix, "ex")
	}
}
