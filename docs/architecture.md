# Architecture

The same `deploy` binary has a CLI mode and a daemon mode. The daemon owns the public HTTP listener and Unix control socket. Global registrations, state, history, and secrets are kept under standard Linux system directories rather than inside repositories.

Webhook handlers cap request size, authenticate HMAC-SHA256 with constant-time comparison, reject non-push events and mismatched repository/branch data, and return after queueing. Deployment workers fetch and confirm the remote commit before changing the checkout. A worker has at most one pending job; the latest replaces any older pending job.

State and pending jobs are atomically persisted as JSON. On startup the daemon reloads pending jobs. History records contain deployment outcome and timing, while webhook and Discord secrets stay in separate `0600` files.
