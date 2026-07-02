package portfolio

import "fmt"

// AccountType represents the type of trading account.
type AccountType string

const (
	AccountSpot    AccountType = "spot"
	AccountMargin  AccountType = "margin"
	AccountFutures AccountType = "futures"
)

// Valid reports whether the account type is recognized.
func (a AccountType) Valid() bool {
	switch a {
	case AccountSpot, AccountMargin, AccountFutures:
		return true
	default:
		return false
	}
}

// String returns the string representation of the account type.
func (a AccountType) String() string {
	return string(a)
}

// ParseAccountType parses a string into an AccountType.
func ParseAccountType(s string) (AccountType, error) {
	at := AccountType(s)
	if !at.Valid() {
		return "", fmt.Errorf("invalid account type: %s", s)
	}
	return at, nil
}
