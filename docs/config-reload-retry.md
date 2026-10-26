# Configuration reload retry

Configuration polling now uses `CheckServerConfig`, which compares a candidate
against the running instance without advancing its ETag/body-hash baseline.
The reload path creates a separate client and core. Only a successful listener
startup replaces the running instance. If preparation or startup fails, the
old instance remains/restores and the next polling cycle offers the candidate
again. Retries follow the configured pull interval (60 seconds when absent or
invalid); this is not an immediate-push or seamless-connection guarantee.

Regression tests cover repeated candidates after an unsuccessful application,
ETag and hash-only responses, and unchanged responses after replacement.

Deployment check: change a test server's protocol port, verify the new listener
and user authentication, then test a temporarily occupied port and release it.
The node should retry without another panel edit or deleting a displayed node.
