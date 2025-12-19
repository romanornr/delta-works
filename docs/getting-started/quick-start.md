# Quick Start

This guide will help you run Delta Works for the first time.

## Prerequisites

Before starting, ensure you have:

1. Delta Works installed ([Installation Guide](installation.md))
2. GoCryptoTrader configured with at least one exchange ([Configuration Guide](configuration.md))
3. QuestDB running on localhost

## Running Delta Works

```bash
# From the project directory
go run main.go
```

Or if you've built the binary:

```bash
./delta-works
```

## What Happens on First Run

1. **Exchange Initialization** - Delta Works loads your GoCryptoTrader configuration and initializes connections to enabled exchanges
2. **Holdings Sync** - Fetches current holdings from all configured exchanges
3. **Withdrawal History** - Syncs withdrawal history from supported exchanges
4. **Data Storage** - All data is stored in QuestDB for historical tracking

## Verifying Data in QuestDB

Open the QuestDB web console at http://localhost:9000 and run:

```sql
-- Check holdings data
SELECT * FROM holdings ORDER BY timestamp DESC LIMIT 10;

-- Check withdrawal data
SELECT * FROM withdrawals ORDER BY timestamp DESC LIMIT 10;
```

## Viewing in Grafana

If you have Grafana set up, import the dashboards from the `grafana_dashboards/` folder to visualize your portfolio data.
