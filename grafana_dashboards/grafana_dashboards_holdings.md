# Grafana Dashboards

This directory contains various Grafana dashboards used for monitoring and analyzing different aspects of the system.

## Dashboards

### Holdings Dashboard

**File:** `holdings.json`

**Description:** This dashboard provides insights into the current holdings managed by the system. It includes various panels that display information such as:

- **Holdings Overview**: A summary of the current holdings, including total value and distribution.
- **Asset Breakdown**: A detailed breakdown of each asset in the portfolio along with their respective values.
- **Historical Performance**: Graphs showing the historical performance of the holdings over time.

**Special Configurations:**

- **Data Source**: Ensure that the Grafana instance is connected to the appropriate data source that stores the holdings information.
- **Refresh Interval**: The dashboard is configured to refresh every minute to provide up-to-date information.
- **Alerting**: Alerts are set up to notify when the value of any asset falls below a certain threshold.

## Adding More Dashboards

To add more dashboards to this directory, follow these steps:

1. Create or export the dashboard JSON file from Grafana.
2. Save the JSON file in this directory.
3. Update this `README.md` file to include a description of the new dashboard, along with any special configurations.

## Example

If you have a dashboard for monitoring CPU usage, it might look like this:

### CPU Usage Dashboard

**File:** `cpu_usage.json`

**Description:** This dashboard monitors CPU usage across different servers. It includes panels such as:

- **CPU Load**: Current load on each server.
- **CPU Utilization**: Graphs showing the percentage of CPU utilization over time.
- **Top Processes**: A list of the top processes consuming the most CPU power.

**Special Configurations:**

- **Data Source**: Ensure the Grafana instance is connected to the server metrics data source.
- **Refresh Interval**: The dashboard is set to refresh every 30 seconds.
- **Alerting**: Alerts are configured to notify high CPU usage.

Feel free to customize the information as per your dashboards and data sources.