# Delta Works

Delta Works is an order execution system (OEMS) for managing and executing orders across multiple exchanges.
Algorithmic trading strategies can be implemented and sophisticated order routing and execution logic can be defined.

Built using [GoCryptoTrader](https://github.com/thrasher-corp/gocryptotrader)
Delta OEMS features including real-time market data, order management, and portfolio tracking.

## Features
### Holdings Management

[x] Real-time tracking of account holdings across multiple exchanges

[x] Support for different account types (spot, margin, futures)

[x] Automatic holdings updates at configurable intervals

[x] Storage of historical holdings data in QuestDB

### Withdrawal Management

[x] Fetch and store withdrawal history from supported exchanges

[x] Batch processing of withdrawal records

[x] Duplicate prevention using timestamp-based tracking

[x] Efficient storage in QuestDB with proper handling of initial sync

[x] Support for all withdrawal types (crypto and fiat)

### Grafana intergration 
[x] Portfolio tracking and visualization

[x] Holdings and withdrawal data visualization


### WIP 
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



Current Directory structure (WIP):
```
deltaworks/
├── cmd/
│   └── main.go                 # Application entry point
├── internal/
│   ├── core/
│   │   ├── core.go            # Core engine functionality
│   │   ├── holdings_manager.go # Holdings management
│   │   ├── withdrawal_manager.go # Withdrawal operations
│   │   ├── exchange_setup.go  # Exchange configuration
│   │   └── portfolio_manager.go # Portfolio management
│   ├── models/
│   │   ├── holdings.go        # Data models for holdings
│   │   └── withdrawal.go      # Data models for withdrawals
│   ├── repository/
│   │   ├── questdb_repository.go # Base QuestDB operations
│   │   ├── holdings.go        # Holdings storage operations
│   │   └── withdrawals.go     # Withdrawal storage operations
│   └── logger/
│       └── logger.go          # Logging configuration
├── config/
│   └── config.json           # Application configuration
├── scripts/
│   └── init_questdb.sql      # Database initialization scripts
├── grafana/
│   ├── dashboards/
│   │   ├── holdings.json     # Holdings visualization
│   │   └── withdrawals.json  # Withdrawal visualization
│   └── queries/
│       ├── holdings_queries.sql
│       └── withdrawal_queries.sql
└── docs/
    └── API.md                # API documentation
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

Dependencies

- **[GoCryptoTrader](https://github.com/thrasher-corp/gocryptotrader)**: Core trading functionality
- **QuestDB**: Time-series database for data storage
- **Zerolog**: Structured logging
- **Chi**: HTTP routing (if applicable)