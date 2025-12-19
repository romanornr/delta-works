# Installation

## Prerequisites

- Go 1.24 or later
- QuestDB (for time-series data storage)
- GoCryptoTrader configuration

## Installing Delta Works

```bash
# Clone the repository
git clone https://github.com/romanornr/delta-works.git
cd delta-works

# Install dependencies
go mod download

# Build the application
go build -o delta-works .
```

## Installing QuestDB

Download the latest release from [QuestDB GitHub Releases](https://github.com/questdb/questdb/releases).

```bash
# Extract
tar -xvf questdb-*-rt-linux-x86-64.tar.gz

# Run QuestDB
./questdb-*/bin/questdb.sh start
```

### Ports
- `9000` - Web Console & REST API
- `9009` - InfluxDB Line Protocol (ILP)
- `8812` - PostgreSQL wire protocol

## Next Steps

- [Configuration](configuration.md) - Set up your configuration file
- [Quick Start](quick-start.md) - Run your first sync
