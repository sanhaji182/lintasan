package lb

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// CreditRank is a provider-agnostic ordering hint for one account.
// Class 0 means expiring credit is available; higher classes are less
// preferred. ExpiresAt/Remaining only matter for class 0.
type CreditRank struct {
	Class     int
	ExpiresAt time.Time
	Remaining float64
}

// AccountCreditRanker supplies a ranking hint for an account ID.
type AccountCreditRanker func(accountID string) CreditRank

// Account represents an API key/account for a provider.
type Account struct {
	ID            string
	APIKey        string
	Priority      int // lower = higher priority (drain primary first)
	Active        bool
	SuccessCount  int64
	FailCount     int64
	RateLimited   bool
	RateLimitedAt time.Time
	Cooldown      time.Duration // how long to skip after rate limit

	// AuthFailed marks an account the upstream rejected with 401/403 — a bad,
	// revoked, or quota-exhausted key. Kept separate from RateLimited because
	// the two mean different things and deserve different cooldowns: a 429
	// clears in seconds, a rejected key does not. It is a cooldown rather than
	// a permanent kill so a key that recovers (daily quota reset, billing
	// fixed) rejoins the rotation without an operator restarting the process.
	AuthFailed   bool
	AuthFailedAt time.Time
	AuthCooldown time.Duration
}

// DefaultAuthCooldown is how long an account stays out of rotation after the
// upstream rejected its key with 401/403.
const DefaultAuthCooldown = 5 * time.Minute

// MultiAccountLB implements round-robin across multiple API key accounts
// for a single provider, with rate-limit awareness and priority drain.
type MultiAccountLB struct {
	mu           sync.RWMutex
	accounts     []Account
	rrIndex      uint64
	providerID   string
	creditRanker AccountCreditRanker
}

// SetCreditRanker installs (or clears) the credit-aware ordering hint.
// When unset, Pick keeps the legacy priority/round-robin behaviour.
func (m *MultiAccountLB) SetCreditRanker(ranker AccountCreditRanker) {
	m.mu.Lock()
	m.creditRanker = ranker
	m.mu.Unlock()
}

// NewMultiAccountLB creates a new multi-account round-robin load balancer.
func NewMultiAccountLB(providerID string, accounts []Account) *MultiAccountLB {
	for i := range accounts {
		if accounts[i].Cooldown == 0 {
			accounts[i].Cooldown = 60 * time.Second // default cooldown
		}
		if accounts[i].AuthCooldown == 0 {
			accounts[i].AuthCooldown = DefaultAuthCooldown
		}
	}
	return &MultiAccountLB{
		providerID: providerID,
		accounts:   accounts,
	}
}

// Pick returns the next available account using round-robin.
// Skips rate-limited accounts (cooldown expired accounts are reactivated).
// Priority: drains primary (lowest priority number) first.
func (m *MultiAccountLB) Pick() (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.accounts) == 0 {
		return nil, fmt.Errorf("multi-account: no accounts for provider %s", m.providerID)
	}

	now := time.Now()
	available := m.availableAccountsLocked(now)
	if len(available) == 0 {
		return nil, fmt.Errorf("multi-account: all accounts for provider %s are rate-limited", m.providerID)
	}

	if m.creditRanker != nil {
		if chosen, ok := m.pickCreditAwareLocked(available); ok {
			return chosen, nil
		}
	}

	// Round-robin among available accounts (sorted by priority)
	rrIdx := atomic.AddUint64(&m.rrIndex, 1) - 1
	chosen := available[rrIdx%uint64(len(available))]
	idx := m.accountIndexLocked(chosen.ID)
	if idx >= 0 {
		return &m.accounts[idx], nil
	}
	return &chosen, nil
}

// pickCreditAwareLocked drains the account with the soonest expiring credit.
// If no account has expiring credit data, it round-robins within the best
// available class (so unknown data keeps the legacy rotation, and zero-credit
// or unhealthy accounts are only used when nothing better remains).
// Must be called with mu held.
func (m *MultiAccountLB) pickCreditAwareLocked(available []Account) (*Account, bool) {
	type ranked struct {
		account Account
		rank    CreditRank
	}
	items := make([]ranked, len(available))
	minClass := 0
	for i, account := range available {
		rank := m.creditRanker(account.ID)
		items[i] = ranked{account: account, rank: rank}
		if i == 0 || rank.Class < minClass {
			minClass = rank.Class
		}
	}
	filtered := items[:0]
	for _, item := range items {
		if item.rank.Class == minClass {
			filtered = append(filtered, item)
		}
	}
	if minClass == 0 {
		sort.SliceStable(filtered, func(i, j int) bool {
			a, b := filtered[i].rank, filtered[j].rank
			if !a.ExpiresAt.Equal(b.ExpiresAt) {
				return a.ExpiresAt.Before(b.ExpiresAt)
			}
			if a.Remaining != b.Remaining {
				return a.Remaining < b.Remaining
			}
			return filtered[i].account.Priority < filtered[j].account.Priority
		})
		if chosen := m.accountPtrLocked(filtered[0].account.ID); chosen != nil {
			return chosen, true
		}
		return nil, false
	}
	rrIdx := atomic.AddUint64(&m.rrIndex, 1) - 1
	if chosen := m.accountPtrLocked(filtered[rrIdx%uint64(len(filtered))].account.ID); chosen != nil {
		return chosen, true
	}
	return nil, false
}

func (m *MultiAccountLB) accountPtrLocked(id string) *Account {
	if idx := m.accountIndexLocked(id); idx >= 0 {
		return &m.accounts[idx]
	}
	return nil
}

// RecordSuccess marks an account as having succeeded.
func (m *MultiAccountLB) RecordSuccess(accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == accountID {
			m.accounts[i].SuccessCount++
			m.accounts[i].Active = true
			break
		}
	}
}

// RecordFailure marks an account as having failed.
func (m *MultiAccountLB) RecordFailure(accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == accountID {
			m.accounts[i].FailCount++
			break
		}
	}
}

// MarkRateLimited marks an account as rate-limited with a cooldown period.
func (m *MultiAccountLB) MarkRateLimited(accountID string) {
	m.MarkRateLimitedWith(accountID, 0)
}

// MarkRateLimitedWith is MarkRateLimited with an explicit cooldown, so the
// B1 classifier (lb.ClassifyUpstreamError, new in the 9router parity plan)
// can drive the expiry from the upstream's own wording. Cooldown 0 keeps each
// account's existing value unchanged — same as MarkRateLimited.
func (m *MultiAccountLB) MarkRateLimitedWith(accountID string, cooldown time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == accountID {
			m.accounts[i].RateLimited = true
			m.accounts[i].RateLimitedAt = time.Now()
			if cooldown > 0 {
				m.accounts[i].Cooldown = cooldown
			}
			break
		}
	}
}

// MarkAuthFailed takes an account out of rotation after the upstream rejected
// its key with 401/403. Distinct from MarkRateLimited: a rejected key is not a
// throughput problem and clearing it in 60s just burns another request on the
// same locked door.
func (m *MultiAccountLB) MarkAuthFailed(accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == accountID {
			m.accounts[i].AuthFailed = true
			m.accounts[i].AuthFailedAt = time.Now()
			if m.accounts[i].AuthCooldown == 0 {
				m.accounts[i].AuthCooldown = DefaultAuthCooldown
			}
			break
		}
	}
}

// AvailableCount reports how many accounts could be picked right now. Callers
// use it to decide whether a pool still has somewhere to go before giving up on
// the connection entirely.
func (m *MultiAccountLB) AvailableCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.availableAccountsLocked(time.Now()))
}

// Accounts returns a snapshot of all accounts.
func (m *MultiAccountLB) Accounts() []Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Account, len(m.accounts))
	copy(out, m.accounts)
	return out
}

// AccountStats returns health stats per account.
func (m *MultiAccountLB) AccountStats() []AccountStatsEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := make([]AccountStatsEntry, len(m.accounts))
	for i, a := range m.accounts {
		total := a.SuccessCount + a.FailCount
		successRate := 0.0
		if total > 0 {
			successRate = float64(a.SuccessCount) / float64(total)
		}
		stats[i] = AccountStatsEntry{
			ID:           a.ID,
			SuccessCount: a.SuccessCount,
			FailCount:    a.FailCount,
			SuccessRate:  successRate,
			RateLimited:  a.RateLimited,
			AuthFailed:   a.AuthFailed,
			Active:       a.Active,
		}
	}
	return stats
}

// AccountStatsEntry is the public view of account health stats.
type AccountStatsEntry struct {
	ID           string
	SuccessCount int64
	FailCount    int64
	SuccessRate  float64
	RateLimited  bool
	AuthFailed   bool
	Active       bool
}

// AddAccount adds a new account to the pool.
func (m *MultiAccountLB) AddAccount(acct Account) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if acct.Cooldown == 0 {
		acct.Cooldown = 60 * time.Second
	}
	if acct.AuthCooldown == 0 {
		acct.AuthCooldown = DefaultAuthCooldown
	}
	m.accounts = append(m.accounts, acct)
}

// RemoveAccount removes an account by ID.
func (m *MultiAccountLB) RemoveAccount(accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == accountID {
			m.accounts = append(m.accounts[:i], m.accounts[i+1:]...)
			break
		}
	}
}

// availableAccountsLocked returns accounts that are active, not rate-limited,
// and not sitting out an auth failure. Must be called with mu held.
func (m *MultiAccountLB) availableAccountsLocked(now time.Time) []Account {
	var available []Account
	for i := range m.accounts {
		a := &m.accounts[i]
		// Auto-reactivate if cooldown has passed
		if a.RateLimited && a.Cooldown > 0 && now.Sub(a.RateLimitedAt) >= a.Cooldown {
			a.RateLimited = false
		}
		// Same for a rejected key: give it another chance once the (longer)
		// auth cooldown expires, since quota resets and billing fixes happen
		// without anyone touching Lintasan.
		if a.AuthFailed && a.AuthCooldown > 0 && now.Sub(a.AuthFailedAt) >= a.AuthCooldown {
			a.AuthFailed = false
		}
		if a.Active && !a.RateLimited && !a.AuthFailed {
			available = append(available, *a)
		}
	}
	// Sort by priority (lower number = higher priority)
	sortAccountsByPriority(available)
	return available
}

func (m *MultiAccountLB) accountIndexLocked(id string) int {
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			return i
		}
	}
	return -1
}

func sortAccountsByPriority(accounts []Account) {
	for i := 1; i < len(accounts); i++ {
		for j := i; j > 0 && accounts[j].Priority < accounts[j-1].Priority; j-- {
			accounts[j], accounts[j-1] = accounts[j-1], accounts[j]
		}
	}
}
