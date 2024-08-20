# Delta Works

Delta Works is an order execution system (OEMS) for managing and executing orders across multiple exchanges.
Algorithmic trading strategies can be implemented and sophisticated order routing and execution logic can be defined.

Built using [GoCryptoTrader](https://github.com/thrasher-corp/gocryptotrader)
Delta OEMS features including real-time market data, order management, and portfolio tracking.

## Features
[x] Multi-exchange support

[ ] Real-time market data

[ ] Portfolio tracking

[ ] Order management

[ ] Algorithmic trading strategies

[ ] Order routing and execution logic

[ ] Arbitrage opportunities

[ ] Twap and Vwap order execution

[ ] iceberg orders

[ ] Stop-loss and take-profit orders

[ ] Backtesting and simulation

[ ] REST and Websocket API

[ ] Web-based dashboard

[ ] Grafana integration

[ ] Grafana dashboards for market data, order management, and portfolio tracking

Directory structure (WIP):
```
deltaworks/
├── internal/
│   ├── core/
│   │   └── core.go  (minimal GoCryptoTrader wrapper)
│   ├── oems/
│   │   ├── oems.go  (main OEMS logic)
│   │   ├── order_manager.go
│   │   ├── execution_engine.go
│   │   └── portfolio_manager.go
│   └── exchange/
│       └── adapter.go  (GoCryptoTrader exchange adapter)
└── cmd/
└── deltaworks/
└── main.go
```

old:
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