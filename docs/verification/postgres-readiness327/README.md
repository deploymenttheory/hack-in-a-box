# PostgreSQL first-start readiness (#327)

The pinned official image starts a temporary Unix-socket-only server while
initialising a fresh database. The old local-socket pg_isready probe can report
healthy during that server, even before the target database exists. Actual
isolated logs showed migration failing at12:58:52.308Z during its shutdown; the
durable TCP server was ready only at12:58:52.411Z.

A disposable pinned-image probe compared local-socket and127.0.0.1 TCP readiness
through first startup: local success while TCP was unavailable was observed,
then TCP became available after initialization (8 samples). The normal Compose
probe now checks TCP, matching the application's connection transport. Its
interval, timeout, retries, pinned image, restart policy and volumes are unchanged.

The corrected normal Compose PostgreSQL service configuration was extracted into
a uniquely named private test project with a fresh database volume. The unchanged
normal production migration image from main ef676fea completed all migrations
through schema56 with exit0. The command uses the normal native migration owner,
not a direct SQL schema edit. Exact non-secret results are in proof.json.

Only that isolated project's containers, volumes and networks were removed, and
zero remaining labelled resources was asserted. The user's existing application
and data were untouched. Original startup/migration/cleanup logs remain private.
This proves fresh database startup and migration, not the entire #125 operator
walkthrough or #120 lifecycle acceptance.
