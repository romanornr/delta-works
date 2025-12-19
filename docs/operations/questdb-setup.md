# QuestDB Setup

QuestDB is a high-performance time-series database used by Delta Works to store holdings and withdrawal data.

## Installation (Linux)

Download the latest release from [QuestDB GitHub Releases](https://github.com/questdb/questdb/releases).

```bash
# Extract
tar -xvf questdb-*-rt-linux-x86-64.tar.gz

# Start QuestDB
./questdb-*/bin/questdb.sh start
```

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 9000 | HTTP | Web Console & REST API |
| 9009 | TCP | InfluxDB Line Protocol (ILP) |
| 8812 | TCP | PostgreSQL wire protocol |

## Web Console

Access the QuestDB web console at: http://localhost:9000

The web console allows you to:
- Run SQL queries
- View table schemas
- Monitor database performance
- Import/export data

## Useful Commands

### Start/Stop QuestDB

```bash
# Start
./questdb.sh start

# Stop
./questdb.sh stop

# Status
./questdb.sh status
```

### Common SQL Queries

```sql
-- List all tables
SHOW TABLES;

-- Check holdings data
SELECT * FROM holdings ORDER BY timestamp DESC LIMIT 100;

-- Check withdrawal data
SELECT * FROM withdrawals ORDER BY timestamp DESC LIMIT 100;

-- Get holdings summary by exchange
SELECT exchange, currency, SUM(total) as total_balance
FROM holdings
WHERE timestamp = (SELECT MAX(timestamp) FROM holdings)
GROUP BY exchange, currency
ORDER BY exchange, currency;

-- Get total holdings value over time (requires price data)
SELECT timestamp, SUM(total) as total_holdings
FROM holdings
WHERE currency = 'BTC'
SAMPLE BY 1h;
```

## Data Directory

QuestDB stores data in the `db` directory within the installation folder. Back up this directory to preserve your data.

## Performance Tips

1. **ILP for ingestion** - Delta Works uses ILP (port 9009) for fast data ingestion
2. **PostgreSQL for queries** - Use port 8812 for complex queries
3. **Partitioning** - QuestDB automatically partitions time-series data by day
4. **Memory** - Allocate sufficient memory for large datasets

## Troubleshooting

### Port already in use

```bash
# Check what's using the port
lsof -i :9000
lsof -i :9009
lsof -i :8812

# Kill the process if needed
kill -9 <PID>
```

### Connection refused

- Ensure QuestDB is running: `./questdb.sh status`
- Check firewall settings
- Verify the correct port is being used
