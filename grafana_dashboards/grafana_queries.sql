/*
Query Name: Asset Allocation with 'Other' Category
Dashboard: Holdings Overview
Panel: Asset Allocation Pie Chart
Description: This query retrieves the latest holdings data,
categorizes assets with less than 1% allocation as 'Other',
and calculates the USD value for each category.
*/
WITH latest_holdings AS (
    SELECT *
    FROM holdings
    WHERE timestamp = (SELECT MAX(timestamp) FROM holdings)
    ),
    holdings_with_total AS (
SELECT
    symbol,
    usd_value,
    SUM(usd_value) OVER () AS total_value
FROM latest_holdings
    ),
    categorized_holdings AS (
SELECT
    CASE
    WHEN (usd_value / total_value) < 0.01 THEN 'Other'
    ELSE symbol
    END AS category,
    SUM(usd_value) AS usd_value
FROM holdings_with_total
GROUP BY
    CASE
    WHEN (usd_value / total_value) < 0.01 THEN 'Other'
    ELSE symbol
    END
    )
SELECT category, usd_value
FROM categorized_holdings
ORDER BY usd_value DESC;

-- ========================================

/*
Query Name: Total Portfolio Value Over Time
Dashboard: Portfolio Performance
Panel: Value Trend Line Chart
Description: This query shows the total portfolio value
over time, allowing us to track overall performance.
*/
SELECT
    timestamp,
    SUM(usd_value) as total_value
FROM holdings
WHERE timestamp >= $__timeFrom() AND timestamp <= $__timeTo()
GROUP BY timestamp
ORDER BY timestamp;

-- ========================================

-- Add more queries as needed...