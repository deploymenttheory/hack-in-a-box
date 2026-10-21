# Operating the foundation

## Probes and metrics

`GET /healthz` checks the HTTP process only, including during a database outage. `GET /readyz` checks PostgreSQL, exact application/River schema compatibility and the private volume's ability to create/remove a probe. Provider availability does not affect liveness or readiness. Responses do not reveal connection strings or storage paths.

`GET /ops/metrics` requires `Authorization: Bearer <HACKBOX_METRICS_TOKEN>`. Use a separate scraper secret and do not expose the endpoint through a public unauthenticated proxy. Metrics contain no person, event, team, path or operation identifiers.

| Signal | Initial alert threshold | Response |
| --- | --- | --- |
| Readiness | 3 failures over 30 seconds | Check PostgreSQL connectivity, migration version and volume permissions/free space. |
| `hackbox_http_errors_total` / `hackbox_http_requests_total` | More than 5% 5xx for 5 minutes with at least 20 requests | Correlate safe logs by request ID; inspect dependency health. |
| `hackbox_http_duration_seconds_sum` / `_count` | Mean response time above 2 seconds for 5 minutes | Inspect database pressure and upload sizes; this summary exposes sum/count, not percentile estimates. |
| `hackbox_due_job_lag_seconds` | Above 120 seconds for 2 minutes | Check worker process, handler deployment, eligibility and retry state. |
| `hackbox_failed_deliveries` | Any increase | Inspect durable operation/attempt records; separate permanent and ambiguous outcomes. |
| `hackbox_ambiguous_deliveries` | Any value above zero | Reconcile against the provider before any retry. |
| `hackbox_recovered_leases_total` | More than 3 recoveries in 10 minutes | Investigate worker crashes, resource limits and provider latency. |

HTTP error/latency counters reset on API restart; job gauges and recovered-lease history derive from PostgreSQL. Tune thresholds against the deployment's expected load. Request logs contain a generated request ID, bounded request category, status and duration. They never log headers, query values, bodies, cookies, tokens or assessment text. OIDC failures are reduced to safe fixed errors.

## Durable delivery contract

The domain transaction writes its operation/outbox intent and calls River `InsertTx`. Operation keys are unique within an event. The operation UUID is the stable external idempotency key, retained with delivery attempts even after River prunes jobs. Enqueue retries must describe the same kind, target and due time. PostgreSQL time precision is normalized to microseconds.

Approval and due time are separate. Revocation/cancellation can modify pending work; a running attempt has already crossed the dispatch boundary and cannot promise to undo an external effect. Domain actions must report a conflict when a pending-only update affects zero rows. `Eligible` checks current domain policy; no secrets belong in River arguments. Unknown handler kinds stay pending until their integration is deployed and surface as due-job lag.

An application lease lasts two minutes and handlers receive a 30-second context. A PostgreSQL advisory guard also remains held throughout dispatch, preventing another live worker from acquiring that operation even if the lease expires. Handlers must respect cancellation and use bounded network calls. After a crash, the second worker checks the provider with the same stable operation key. Only an explicit proof that retry is safe may return `Retryable`; missing provider evidence yields `Ambiguous`.

This is at-least-once execution. A unique job does not prove exactly-once external delivery. A process may die after the provider accepted a send but before PostgreSQL recorded its outcome. Ambiguous or permanent operations are retained and never automatically resent. A future integration's reconciliation workflow must resolve these records explicitly through its domain policy and audit trail.

## Files and recovery

Back up PostgreSQL and the files volume as one consistent installation. File contents are private, opaque objects; metadata includes checksum, size, event and owner. A database commit with an unknown outcome leaves the object in place until reconciliation can safely prove it is unreferenced. The worker checks for unreferenced objects older than 24 hours at startup and every minute, and stops cleanup on database errors.

Never edit an audit record or delete a delivered operation to clear an alert. Fix the underlying condition and retain the evidence. No public API provides audit mutation. Event audit query/export policy is enforced server-side; current event organiser membership is required, including for platform administrators.

For schema upgrades, stop API and worker before migrating. The dedicated migration connection holds a PostgreSQL session lock across the complete Goose and River upgrade; a disconnected migrator releases it. A failed migration can be rerun, but a failed deployment must not start incompatible binaries. Keep the previous image and matching backup until validation succeeds.
