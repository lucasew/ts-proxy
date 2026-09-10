# ts-proxy

One process joins a tailnet as one or more nodes and publishes reachable TCP (and HTTP) services under those names.

Status: approved

The key words MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY in this
document are to be interpreted as described in BCP 14
(RFC 2119, RFC 8174) when, and only when, they appear in all
capitals.

Genre: cli (plus an importable Go package used only by this binary)

## Intention

Job: give each configured service its own Tailscale identity (`hostname.ts.net`) without installing `tailscaled` per service.

Non-goals (this version):

- UDP catch-all
- Kernel TUN / iptables DNAT / true L3 wrap
- Funnel on ports other than the Tailscale Funnel set
- Preserving the tailnet peer address on the upstream socket

Tree-bound: `cmd/ts-proxyd`, `pkg/config`, `pkg/server`, `pkg/handler`, `example-config.yaml`.

## Technique

| ID | Input | Rule | Output |
|----|-------|------|--------|
| TEC-1 | YAML server + auth | One `tsnet.Server` per server slug, state under `state_dir/<slug>` | A tailnet node named `hostname` |
| TEC-2 | Handler `listen` + `upstream_address` | `Listen` / `ListenTLS` / `ListenFunnel` then splice or HTTP reverse proxy | Port-specific publish |
| TEC-3 | Server `forward` host | `RegisterFallbackTCPHandler` after listeners; dest port N dials `forward:N` | Unmatched inbound TCP published |

TEC-3 wraps `tailscale.com/tsnet.Server.RegisterFallbackTCPHandler`. Listeners always win.

## Tooling

| Technique | Relation | Cite |
|-----------|----------|------|
| TEC-1, TEC-2, TEC-3 | adopt | `tailscale.com/tsnet` v1.102.3 |
| YAML + flags + env | adopt | cobra, viper |
| HTTP reverse proxy + WhoIs headers | implement | `pkg/handler/http.go` (same header set as Tailscale serve / tclip) |
| TCP splice | implement | `pkg/handler/tcp.go` |

## Terminology

| Concept | Approved | Banned |
|---------|----------|--------|
| One tsnet identity | server | node (except “Tailscale node” in prose), vhost |
| Port-specific publish | handler | route, mapping, endpoint |
| Host for unmatched TCP | forward | wrap, dest IP, catch-all, proxy_to |
| Inbound dest port reused on forward | dest port | published port, listen port (that name is the handler bind) |

## Types

| Type | Exported | Identity or value | Mutable | Nil/error | Callers MUST NOT |
|------|----------|-------------------|---------|-----------|------------------|
| `config.Config` | yes | value | after load, no | `Validate` error | mutate while `Supervisor` runs |
| `config.ServerConfig` | yes | slug key | no | see Errors | set `forward` to `host:port` |
| `config.HandlerConfig` | yes | `listen` on that server | no | missing listen/upstream | share `listen` on one server |
| `server.Server` | yes | slug | lifecycle SM | `ErrNotStarted` before `Start` | `Serve` before `Start` |

## Commands

| Command | Type it mutates | Transition | Bad input |
|---------|-----------------|------------|-----------|
| `ts-proxyd server` | each `Server` | start → authenticate → run | load/validate fail: exit 1, no nodes left running |
| `ts-proxyd config` | none | print resolved YAML | same load/validate fail: exit 1 |
| `ts-proxyd server --dry-run` | each `Server` (auth only) | start then close | start fail: close earlier successes, exit 1 |

## Invariants

- INV-1 A server slug and token slug MUST match `^[a-zA-Z0-9_]+$`.
- INV-2 A handler `listen` on one server MUST be unique.
- INV-3 `forward` MUST be empty or a host with no port (IPv4, unbracketed IPv6, or hostname without `:`).
- INV-4 When a handler listens on dest port N, inbound TCP to N MUST go to that handler, never to `forward`.
- INV-5 When `forward` is set and no handler owns dest port N, inbound TCP to N MUST be dialed as `forward:N` over TCP.
- INV-6 When `forward` is empty, unmatched inbound TCP MUST be rejected.
- INV-7 A server with no handlers and no `forward` MUST stay running and MUST reject inbound TCP. The process SHOULD log a warning. Exception: `--dry-run` never serves, so it does not log that warning.
- INV-8 `forward` MUST NOT accept UDP or non-TCP IP protocols.
- INV-9 Funnel MUST exist only through an explicit handler (`ListenFunnel`).
- INV-10 HTTP handlers MUST add Tailscale WhoIs headers. TCP splice and `forward` MUST NOT invent those headers.

## Errors

| Operation | Bad input | Reaction |
|-----------|-----------|----------|
| `Config.Validate` | `forward` contains a port | `ErrForwardHostOnly`, load fails |
| `Config.Validate` | unknown handler type | `ErrUnknownHandlerType` |
| `Config.Validate` | handler missing listen/upstream | `ErrListenRequired` / `ErrUpstreamRequired` |
| `Config.ExpandEnv` | `${UNSET}` | `ErrUndefinedEnvVar` |
| `Server.Serve` | `Start` not called | `ErrNotStarted` |
| fallback dial | `forward:N` unreachable | close inbound conn, log, keep serving |

## Success

- `forward: 127.0.0.2` on hostname `gremio` makes `gremio.<tailnet>:22` reach `127.0.0.2:22` over TCP.
- The same server with a handler on `:80` still serves that handler on port 80.
- Omitting `forward` leaves unmatched ports closed.
- `ts-proxyd config` prints the resolved `forward` value.

## Later work

1. UDP unmatched-port publish (no public tsnet hook today).
2. Kernel L3 DNAT (`TS_DEST_IP` shape) for ICMP and other IP protocols.
3. PROXY protocol or TPROXY so the upstream sees the tailnet peer.
