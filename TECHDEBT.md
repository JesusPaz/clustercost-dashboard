# Tech Debt

- Add a feature flag to disable high-cardinality connection labels (src/dst IPs, services) or aggregate them before ingesting into VictoriaMetrics.
- Confirm whether connection byte/cost metrics are counters or per-interval deltas; adjust topology query to use `increase()` vs `sum_over_time()` accordingly.
- Consider adding a query_range-based topology endpoint for true historical windows instead of single-point `increase(@ end)`.
