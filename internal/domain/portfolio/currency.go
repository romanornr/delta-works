package portfolio

import "strings"

// stablecoins contains common stablecoins pegged to USD
var stablecoins = map[string]bool{
	"USDT": true,
	"USDC": true,
	"BUSD": true,
	"DAI":  true,
	"TUSD": true,
	"USDP": true,
	"GUSD": true,
	"FRAX": true,
}

// IsStablecoin reports whether the currency is a stablecoin
func IsStablecoin(currency string) bool {
	return stablecoins[strings.ToUpper(currency)]
}

// IsUSD reports whether the asset is USD
func IsUSD(asset string) bool {
	return strings.ToUpper(asset) == "USD"
}

// IsUSDEquivalent reports whether the asset is USD or a stablecoin
func IsUSDEquivalent(asset string) bool {
	return IsUSD(asset) || IsStablecoin(asset)
}

// NormalizeAsset normalizes an asset code
func NormalizeAsset(asset string) string {
	return strings.ToUpper(strings.TrimSpace(asset))
}
