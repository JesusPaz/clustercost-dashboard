# Technical Debt & Roadmap

## High Priority
- [ ] **High Cardinality Optimization**: Add a feature flag or logic to disable high-cardinality connection labels (e.g., specific src/dst IPs, ephemeral ports) or aggregate them (e.g., `external-traffic`) before ingestion into VictoriaMetrics to prevent index explosion.
- [ ] **Metric Type Definition**: Confirm definitively whether connection byte/cost metrics are cumulative **counters** or per-interval **deltas/gauges**.
    - If Counters: Queries must use `increase()`.
    - If Gauges: Queries must use `sum_over_time()` or `avg_over_time()`.
    - *Action*: Standardize on Counters for consistency with Prometheus best practices.

## medium Priority
- [ ] **Historical Topology API**: Implement a `query_range`-based endpoint for topology data.
    - Current: Single-point `increase(@ end)` (Snapshot).
    - Desired: Dynamic windowing for "Network usage over the last 7 days".

## Future Considerations
- [ ] **Retention Policies**: Configure distinct retention periods for high-precision metrics (15s interval) vs. aggregated historical data.
- [ ] **Refactor Store Locking**: Evaluate moving from heavy `RWMutex` usage in `store.go` to a more concurrent pattern if contention increases with 100+ agents.
