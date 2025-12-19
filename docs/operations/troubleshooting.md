# Troubleshooting

Common issues and solutions for Delta Works.

## QuestDB Connection Issues

### "failed to create QuestDB sender"

**Cause:** QuestDB is not running or not accessible on the expected port.

**Solution:**
```bash
# Check if QuestDB is running
./questdb.sh status

# Start QuestDB if not running
./questdb.sh start

# Verify port 9009 is listening
lsof -i :9009
```

### "failed to ping database"

**Cause:** PostgreSQL wire protocol connection failed (port 8812).

**Solution:**
```bash
# Check if port 8812 is available
lsof -i :8812

# Test connection manually
psql -h localhost -p 8812 -U admin -d qdb
```

## Exchange Issues

### "exchange not found"

**Cause:** The exchange is not enabled in GoCryptoTrader configuration.

**Solution:**
1. Open `~/.gocryptotrader/config.json`
2. Find the exchange section
3. Set `"enabled": true`
4. Restart Delta Works

### "authentication failed"

**Cause:** Invalid API credentials or missing permissions.

**Solution:**
1. Verify API key and secret are correct
2. Check API key permissions on the exchange website
3. Some exchanges require IP whitelisting
4. Ensure `"authenticatedSupport": true` in config

### Holdings not updating

**Cause:** Exchange API rate limits or connection issues.

**Solution:**
1. Check the logs for specific error messages
2. Verify the exchange is reachable
3. Wait for rate limit cooldown if applicable

## Data Issues

### Missing historical data

**Cause:** Initial sync may not fetch all historical data.

**Solution:**
- Withdrawal history is fetched from the earliest available date
- Holdings are tracked from when Delta Works starts running
- Historical data before first run is not available

### Duplicate entries

**Cause:** Should not happen - Delta Works uses timestamp-based deduplication.

**Solution:**
```sql
-- Check for duplicates in QuestDB
SELECT exchange, currency, timestamp, COUNT(*)
FROM holdings
GROUP BY exchange, currency, timestamp
HAVING COUNT(*) > 1;
```

## Build Issues

### "go mod download" fails

**Cause:** Network issues or Go module proxy problems.

**Solution:**
```bash
# Try direct download
GOPROXY=direct go mod download

# Or use a different proxy
GOPROXY=https://proxy.golang.org go mod download
```

### Build errors after updating dependencies

**Solution:**
```bash
# Clean module cache
go clean -modcache

# Re-download dependencies
go mod download

# Tidy up go.mod
go mod tidy
```

## Logs

Delta Works uses zerolog for structured logging. Check the console output for detailed error messages.

### Increasing log verbosity

The log level can be adjusted in the logger configuration. Look for debug-level logs for more detailed information.
