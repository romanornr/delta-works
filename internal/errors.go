package internal

import "errors"

// Exchange errors
var (
	// ErrExchangeNotFound is returned when a requested exchange is not configured or available.
	ErrExchangeNotFound = errors.New("exchange not found")

	// ErrExchangeNotEnabled is returned when an exchange exists but is not enabled.
	ErrExchangeNotEnabled = errors.New("exchange not enabled")

	// ErrNoExchangesEnabled is returned when no exchanges are enabled in the configuration.
	ErrNoExchangesEnabled = errors.New("no exchanges enabled")

	// ErrAdapterNotReady is returned when an exchange adapter has not completed initialization.
	ErrAdapterNotReady = errors.New("adapter not ready")
)

// Account errors
var (
	// ErrInvalidAccountType is returned when an unrecognized account type is provided.
	ErrInvalidAccountType = errors.New("invalid account type")

	// ErrAccountNotFound is returned when a requested account does not exist.
	ErrAccountNotFound = errors.New("account not found")
)

// Storage errors
var (
	// ErrStorageUnavailable is returned when the storage backend cannot be reached.
	ErrStorageUnavailable = errors.New("storage unavailable")

	// ErrNoSnapshotsFound is returned when no portfolio snapshots exist for the query.
	ErrNoSnapshotsFound = errors.New("no snapshots found")

	// ErrNoTransfersFound is returned when no transfers exist for the query.
	ErrNoTransfersFound = errors.New("no transfers found")
)

// Transfer errors
var (
	// ErrDuplicateTransfer is returned when attempting to store a transfer that already exists.
	ErrDuplicateTransfer = errors.New("duplicate transfer")

	// ErrInvalidTransfer is returned when a transfer has invalid or missing required fields.
	ErrInvalidTransfer = errors.New("invalid transfer")
)

// Price errors
var (
	// ErrPriceUnavailable is returned when price data cannot be fetched for an asset.
	ErrPriceUnavailable = errors.New("price data unavailable")

	// ErrTickerNotFound is returned when ticker data is not available for a trading pair.
	ErrTickerNotFound = errors.New("ticker not found")

	// ErrInvalidPair is returned when a trading pair format is invalid.
	ErrInvalidPair = errors.New("invalid trading pair")
)

// Configuration errors
var (
	// ErrConfigInvalid is returned when configuration validation fails.
	ErrConfigInvalid = errors.New("configuration invalid")

	// ErrConfigNotFound is returned when the configuration file cannot be found.
	ErrConfigNotFound = errors.New("configuration file not found")
)

// General errors
var (
	// ErrInvalidCurrency is returned when a currency code is invalid or unrecognized.
	ErrInvalidCurrency = errors.New("invalid currency")

	// ErrInvalidAmount is returned when an amount is negative or otherwise invalid.
	ErrInvalidAmount = errors.New("invalid amount")
)

// IsNotFound returns true if the error is a "not found" type error.
// This is useful for handling missing data gracefully.
func IsNotFound(err error) bool {
	switch {
	case errors.Is(err, ErrExchangeNotFound),
		errors.Is(err, ErrAccountNotFound),
		errors.Is(err, ErrNoSnapshotsFound),
		errors.Is(err, ErrNoTransfersFound),
		errors.Is(err, ErrTickerNotFound),
		errors.Is(err, ErrConfigNotFound):
		return true
	}
	return false
}
