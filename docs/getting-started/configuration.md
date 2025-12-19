# Configuration

## GoCryptoTrader Configuration

Delta Works uses GoCryptoTrader's configuration system. The configuration file is typically located at:

- Linux: `~/.gocryptotrader/config.json`
- macOS: `~/.gocryptotrader/config.json`
- Windows: `%APPDATA%\GoCryptoTrader\config.json`

## Exchange Configuration

Each exchange needs to be configured with API credentials in the GoCryptoTrader config file.

### Example Exchange Configuration

```json
{
  "name": "Kraken",
  "enabled": true,
  "verbose": false,
  "api": {
    "authenticatedSupport": true,
    "authenticatedWebsocketApiSupport": false,
    "credentials": {
      "key": "YOUR_API_KEY",
      "secret": "YOUR_API_SECRET"
    }
  }
}
```

## QuestDB Connection

Delta Works connects to QuestDB using two protocols:

1. **InfluxDB Line Protocol (ILP)** on port `9009` - For high-performance data ingestion
2. **PostgreSQL wire protocol** on port `8812` - For queries and meta operations

### Default Connection Settings

```
Host: localhost
ILP Port: 9009
PostgreSQL Port: 8812
User: admin
Password: quest
Database: qdb
```

> **Note:** Connection settings are currently hardcoded in the repository. Configuration via environment variables is planned for a future release.

## Next Steps

- [Quick Start](quick-start.md) - Run your first sync
