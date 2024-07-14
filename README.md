# delta-works


Format this into readme.md file

# Delta Works

Delta Works is a trading bot to make trades on Bybit, Binance and other exchanges. 
Directory structure (WIP):

```
deltaWorks/
├── cmd/
│   └── main.go
├── internal/
│   ├── delta/
│   │   ├── core.go
│   │   └── core_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── exchange/
│   │   ├── exchange.go
│   │   ├── binance.go
│   │   ├── coinbase.go
│   │   └── exchange_test.go
│   ├── strategy/
│   │   ├── strategy.go
│   │   ├── simple_strategy.go
│   │   └── strategy_test.go
│   ├── order/
│   │   ├── order.go
│   │   └── order_test.go
│   ├── portfolio/
│   │   ├── portfolio.go
│   │   └── portfolio_test.go
│   └── util/
│       ├── logger.go
│       └── math.go
├── pkg/
│   └── indicator/
│       ├── indicator.go
│       ├── moving_average.go
│       └── rsi.go
├── config/
│   └── config.json
├── scripts/
│   ├── backtest.sh
│   └── deploy.sh
├── docs/
│   ├── README.md
│   └── API.md
├── go.mod
└── go.sum
```