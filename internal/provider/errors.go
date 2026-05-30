package provider

import "errors"

// Sentinel errors the SDK and its callers can match with errors.Is. Keeping
// these explicit (rather than ad-hoc fmt.Errorf strings) means the router can
// branch on failure mode later without string matching.
var (
	// ErrNotRegistered is returned by MustGet when a name has no provider.
	ErrNotRegistered = errors.New("provider: not registered")

	// ErrNilProvider is returned by Register when handed a nil Provider.
	ErrNilProvider = errors.New("provider: cannot register nil provider")

	// ErrEmptyName is returned by Register when a provider reports an empty Name().
	ErrEmptyName = errors.New("provider: cannot register provider with empty name")

	// ErrPrepare wraps failures originating in Provider.Prepare.
	ErrPrepare = errors.New("provider: prepare failed")

	// ErrTranslate wraps failures originating in Provider.Translate.
	ErrTranslate = errors.New("provider: translate failed")
)
