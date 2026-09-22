package server

// qoder_bulk_pat.go — add many Qoder PATs in one call.
//
// Why this exists: the generic `POST /api/connections` takes exactly one credential
// and requires base_url / chat_path / models_path / auth_header / auth_prefix to be
// supplied. Qoder needs the same six values on every row, so adding a batch by hand
// means repeating them N times — and getting one wrong produces a connection that
// authenticates but routes nowhere.
//
// This endpoint takes bare credentials and derives everything else, because every
// Qoder connection is identical except for the PAT and its priority.
//
// The label is read from the account itself rather than invented, so a row is
// identifiable in the dashboard. The jobToken exchange that precedes every session
// returns `{ id, name, plan, userTag }`; the id's tail doubles as the account handle
// in Qoder's own tooling. The resulting name matches what the earlier import produced
// ("Qoder b46ac2eeef5aRyanPerry"), so a batch added here is indistinguishable from one
// imported from qoder2api.
//
// POST /api/qoder/credentials
//   { "pats": "pt-aaa, pt-bbb\npt-ccc", "validate": true, "priority_start": 60,
//     "pool_id": "", "prefix": "Qoder" }
//
// `pats` accepts newline-, comma-, semicolon- or whitespace-separated input and
// tolerates duplicates and stray quotes, because it is meant to be pasted.
//
// Response is per-credential: what was added, what was skipped and why. A partial
// failure never aborts the batch — one dead PAT must not cost an operator the other
// fifty.
//
// Validation is ON by default. It costs one session exchange per credential, which is
// the same call chat makes, so it is cheap; and a credential that cannot authenticate
// is worth rejecting at paste time rather than discovering later as a connection that
// 401s on every request.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// qoderConnectionBaseURL is the base every Qoder row uses. Kept as a constant so the
// value cannot drift between the bulk path and the preset catalogue.
const qoderConnectionBaseURL = "https://api.qoder.com/v1"

// qoderPATShape is the credential format Qoder issues: "pt-" followed by a long
// URL-safe token. Used as a pre-flight so an obviously malformed paste is rejected
// before it can be written, and rejected identically whether or not validation is on.
//
// Why this matters: with validation OFF (an operator opting for speed on a large
// batch), a mistyped or truncated value would otherwise be inserted verbatim and
// surface later as a connection that 401s on every request — the exact failure the
// validate-on-by-default setting exists to prevent. A shape check costs nothing and
// catches the common case (paste truncation, wrong column, stray prose).
var qoderPATShape = regexp.MustCompile(`^pt-[A-Za-z0-9_-]{16,}$`)

// qoderBulkCredential is one input row, after parsing.
type qoderBulkCredential struct {
	Raw      string `json:"raw"`
	Redacted string `json:"redacted"`
}

// splitQoderPATs parses pasted credentials.
//
// Deliberately permissive: operators paste from a spreadsheet column, a CSV, shell
// history, or a chat message. Anything that separates tokens is accepted, and
// surrounding quotes and whitespace are stripped, because a parse failure for a
// cosmetic reason is worse than a slightly loose parser.
func splitQoderPATs(raw string) []qoderBulkCredential {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '\n', '\r', '	', ';', ' ', '"', '\'':
			return true
		}
		return false
	})

	seen := make(map[string]bool, len(fields))
	out := make([]qoderBulkCredential, 0, len(fields))
	for _, f := range fields {
		tok := strings.TrimSpace(f)
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, qoderBulkCredential{Raw: tok, Redacted: redactPAT(tok)})
	}
	return out
}

// redactPAT renders a credential safe to echo back or log: enough to identify it,
// never enough to use it.
func redactPAT(p string) string {
	r := strings.TrimSpace(p)
	if len(r) <= 10 {
		return "***"
	}
	return r[:6] + "…" + r[len(r)-4:]
}

// qoderAccountLabel derives a display label from the account itself.
//
// The uid tail plus the account name, with spaces removed so the label stays one
// token — the same shape the earlier import produced.
func qoderAccountLabel(uid, name string) string {
	uid = strings.TrimSpace(uid)
	name = strings.Join(strings.Fields(strings.TrimSpace(name)), "")
	if uid == "" {
		return name
	}
	tail := strings.ReplaceAll(uid, "-", "")
	if len(tail) > 12 {
		tail = tail[len(tail)-12:]
	}
	return tail + name
}

// qoderIdentityFromCredential resolves an account label for a credential by running
// the real session exchange.
//
// Returns ("", "") when the credential cannot authenticate, which the caller treats as
// invalid. The exchange is the same one chat performs, so a pass here means the
// credential genuinely works — not merely that it is well-formed.
func (s *Server) qoderIdentityFromCredential(ctx context.Context, credential string) (uid, name string, err error) {
	salt := strings.TrimSpace(os.Getenv("LINTASAN_QODER_SALT"))
	region := strings.TrimSpace(os.Getenv("LINTASAN_QODER_REGION"))
	if region == "" {
		region = "global"
	}
	// A dedicated manager so a validation probe cannot disturb the live session cache.
	probe := qoder.NewSessionManager(salt, region, nil)
	ident, err := probe.IdentityFor(ctx, credential)
	if err != nil {
		return "", "", err
	}
	return ident.UID, ident.Name, nil
}

// handleQoderBulkAdd adds many Qoder PATs as connections.
func (s *Server) handleQoderBulkAdd(w http.ResponseWriter, r *http.Request) {
	if s.proxy.qoderProvider == nil {
		writeJSON(w, map[string]any{
			"success": false, "status": "not_enabled",
			"message": "the Qoder provider is not active; enable qoder_enabled and provision the request template first",
		})
		return
	}

	var input struct {
		PATs          string `json:"pats"`
		PATsSnake     string `json:"pat_list"`
		Validate      *bool  `json:"validate"`
		PriorityStart int    `json:"priority_start"`
		PoolID        string `json:"pool_id"`
		Prefix        string `json:"prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"success": false, "message": "invalid JSON"})
		return
	}
	if strings.TrimSpace(input.PATs) == "" {
		input.PATs = input.PATsSnake
	}

	creds := splitQoderPATs(input.PATs)
	if len(creds) == 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "no credentials found in `pats`",
		})
		return
	}

	// Default to validating: a PAT that cannot authenticate should be rejected now, not
	// discovered later as a connection that 401s on every request.
	validate := true
	if input.Validate != nil {
		validate = *input.Validate
	}
	if input.Prefix == "" {
		input.Prefix = "Qoder"
	}

	// Priorities descend from the start value, matching how existing rows are ordered
	// (highest first in the dashboard).
	priority := input.PriorityStart
	if priority == 0 {
		// Below the current lowest, so an appended batch does not silently outrank the
		// accounts already in place.
		var lowest int
		_ = s.db.Conn().QueryRow(
			`SELECT COALESCE(MIN(priority), 61) FROM connections WHERE LOWER(format) = 'qoder'`,
		).Scan(&lowest)
		priority = lowest - 1
	}

	// Existing credentials are skipped rather than duplicated: re-pasting a list you
	// already added must be a no-op, not a pile of duplicate connections.
	existing := make(map[string]bool)
	if rows, err := s.db.Conn().Query(
		`SELECT api_key FROM connections WHERE LOWER(format) = 'qoder'`); err == nil {
		for rows.Next() {
			var k string
			if rows.Scan(&k) == nil {
				existing[strings.TrimSpace(k)] = true
			}
		}
		rows.Close()
	}

	type rowResult struct {
		Credential string `json:"credential"`
		Status     string `json:"status"` // added | duplicate | invalid | error
		Connection string `json:"connection_id,omitempty"`
		Name       string `json:"name,omitempty"`
		Account    string `json:"account,omitempty"`
		Message    string `json:"message,omitempty"`
	}

	results := make([]rowResult, 0, len(creds))
	added, duplicates, invalid, failed := 0, 0, 0, 0

	for _, c := range creds {
		res := rowResult{Credential: c.Redacted}

		if existing[c.Raw] {
			res.Status = "duplicate"
			res.Message = "already present in this deployment"
			duplicates++
			results = append(results, res)
			continue
		}

		// Shape pre-flight, applied whether or not validation is on. A malformed value
		// written verbatim becomes a connection that 401s on every request; catching it
		// here costs nothing and keeps "validate: false" from meaning "accept anything".
		if !qoderPATShape.MatchString(c.Raw) {
			res.Status = "invalid"
			res.Message = "not a Qoder PAT (expected \"pt-\" followed by a long token)"
			invalid++
			results = append(results, res)
			continue
		}

		label := ""
		if validate {
			ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
			uid, name, err := s.qoderIdentityFromCredential(ctx, c.Raw)
			cancel()
			if err != nil {
				// A rejected credential is a data problem the operator can fix, so it is
				// reported as invalid rather than as a server error.
				res.Status = "invalid"
				res.Message = err.Error()
				invalid++
				results = append(results, res)
				continue
			}
			label = qoderAccountLabel(uid, name)
		}

		connName := strings.TrimSpace(input.Prefix + " " + label)
		if label == "" {
			connName = strings.TrimSpace(input.Prefix + " " + c.Redacted)
		}
		// Names must be unique per deployment or the dashboard becomes ambiguous.
		var clash int
		_ = s.db.Conn().QueryRow(`SELECT COUNT(*) FROM connections WHERE name = ?`, connName).Scan(&clash)
		if clash > 0 {
			connName = connName + "-" + shortSuffix(c.Raw)
		}

		id := uuid.New().String()
		_, err := s.db.Conn().Exec(
			`INSERT INTO connections
			 (id, name, base_url, api_key, oauth_provider, format, priority,
			  chat_path, models_path, auth_header, auth_prefix, pool_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, connName, qoderConnectionBaseURL, c.Raw, "", "qoder", priority,
			"/chat/completions", "/models", "Authorization", "Bearer ",
			strings.TrimSpace(input.PoolID),
		)
		if err != nil {
			res.Status = "error"
			res.Message = err.Error()
			failed++
			results = append(results, res)
			continue
		}

		res.Status = "added"
		res.Connection = id
		res.Name = connName
		res.Account = label
		added++
		existing[c.Raw] = true
		priority--
		results = append(results, res)
	}

	if added > 0 && strings.TrimSpace(input.PoolID) != "" {
		s.proxy.RefreshMultiAccountPools()
	}

	// Stable ordering so a re-run diffs cleanly.
	sort.SliceStable(results, func(i, j int) bool { return results[i].Status < results[j].Status })

	writeJSON(w, map[string]any{
		"success":   added > 0 || (invalid == 0 && failed == 0),
		"validated": validate,
		"summary": map[string]any{
			"submitted":  len(creds),
			"added":      added,
			"duplicates": duplicates,
			"invalid":    invalid,
			"failed":     failed,
			"next_step":  "GET /api/connections/sync discovers models for the new rows",
		},
		"data": results,
	})
}

// shortSuffix gives a name collision a stable, non-secret disambiguator.
func shortSuffix(pat string) string {
	h := 0
	for _, r := range pat {
		h = (h*31 + int(r)) & 0xffffff
	}
	const hex = "0123456789abcdef"
	out := make([]byte, 4)
	for i := 3; i >= 0; i-- {
		out[i] = hex[h&0xf]
		h >>= 4
	}
	return string(out)
}
