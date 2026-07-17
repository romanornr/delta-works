package gct

// Support records which port contracts a GCT venue implementation satisfies.
type Support struct {
	Account       bool
	MarketData    bool
	Orders        bool
	PrivateEvents bool
}

// This table was audited against github.com/romanornr/gocryptotrader at
// fa94ed8d0137084315f909475c23f902f13d43f0, the revision selected by go.mod.
// GCT feature flags do not fully describe wrapper behavior, so dependency
// updates must re-audit the source paths behind every enabled contract.
var supportByVenue = map[string]Support{
	"binanceus":   {Account: true, MarketData: true},
	"binance":     {Account: true, MarketData: true},
	"bitfinex":    {Account: true, MarketData: true},
	"bitflyer":    {MarketData: true},
	"bithumb":     {Account: true, MarketData: true},
	"bitmex":      {Account: true, MarketData: true},
	"bitstamp":    {Account: true, MarketData: true},
	"btc markets": {Account: true, MarketData: true},
	"btse":        {Account: true, MarketData: true},
	"bybit":       {Account: true, MarketData: true},
	"coinut":      {Account: true, MarketData: true},
	"deribit":     {Account: true, MarketData: true},
	"exmo":        {Account: true, MarketData: true},
	"coinbase":    {Account: true, MarketData: true, Orders: true, PrivateEvents: true},
	"gateio":      {Account: true, MarketData: true},
	"gemini":      {Account: true, MarketData: true},
	"hitbtc":      {Account: true, MarketData: true},
	"huobi":       {Account: true, MarketData: true},
	"kraken":      {Account: true, MarketData: true},
	"kucoin":      {Account: true, MarketData: true},
	"lbank":       {Account: true, MarketData: true},
	"okx":         {Account: true, MarketData: true},
	"poloniex":    {Account: true, MarketData: true},
	"yobit":       {Account: true, MarketData: true},
}

// SupportFor returns the audited support for an exact GCT exchange name.
func SupportFor(name string) (Support, bool) {
	support, ok := supportByVenue[name]
	return support, ok
}
