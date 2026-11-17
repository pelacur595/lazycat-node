# Official v1.2.0 + HY2 port hopping

Upstream: perfect-panel/ppanel-node tag v1.2.0, commit
`8bd89c328519eacada5c4bfae60725b671a5df36`.

This branch starts from upstream, rather than reverting selected pieces of the
previous LazyCat fork. User management, device admission, limiter behavior,
protocol implementations and Go dependencies remain upstream versions.

Runtime changes relative to that tag are restricted to:

- `common/porthop`: isolated nftables/iptables UDP redirects.
- `node/porthop.go`: derive and validate hopping ranges from server protocols.
- `node/node.go`: prepare, start, stop and clean up hopping rules.

Additional changes are hopping tests, documentation, firewall dependency
installation, installer/update URLs pointing to pelacur595/lazycat-node, and CI
checks for hopping. There is no custom credential WebSocket notification.

## Backend requirement

Upstream requires authenticated `GET /v1/server/alivelist` at startup and uses
`POST /v1/server/device` to admit IPs of subscriptions with device limits.
An HTTP error is not permission to connect. Online reports must also have the
matching lease-renewal behavior. Do not deploy against the legacy LazyCat backend
without validating these contracts; adding an endpoint which always permits
access is not an implementation of the upstream device-limit behavior.

The previous LazyCat source is preserved in branch
`backup/lazycat-before-official-hy2-20261117` at commit `d2f32cd`.

This branch is not a claim that every upstream bug is fixed. In particular the
pinned Xray dependency is unchanged from v1.2.0.
