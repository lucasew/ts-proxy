# ts-proxy

Simple proxy program to allow exposing individual services to a Tailnet, and
even to the Internet using Tailscale Funnel.

Unfortunately, the Tailscale daemon only allows exposing services using the
current node domain, and you can't spawn (so far) nodes for services. With this
you can!

On first run, each configured server (Tailscale node) may need to be authenticated
with your Tailscale account. You can supply an auth key via the environment (see
`auth_key: "${TS_AUTHKEY}"` in the config file) or follow the login URL printed
on first start.

After the initial login, Tailscale stores certificates and node state under the
`state_dir` configured in the YAML file (a subdirectory is created per server
slug). Subsequent starts normally require no interaction.

As the [main build of Tailscale](https://tailscale.com/s/serve-headers), you
can get information about the user accessing the service using the following
headers that get forwarded to the upstream service:
- `Tailscale-User-Login`
- `Tailscale-User-Name`
- `Tailscale-User-Profile-Pic`

And yeah, you can use Tailscale as a single sign on and have a public facing
version! It's as safe and stable as
[tclip](https://github.com/tailscale-dev/tclip) is because this proxy uses the
exact same primitives.

> [!WARNING]
> You can count on the headers sent by ts-proxy as long as you follow the following conditions:
> - Anything that changes the headers name representation such as Apache with PHP could be cheated
> by passing the header TAILSCALE_USER_LOGIN, for example.
>
> - If some users can access your actual service directly without passing the traffic through ts-proxy
they can change all the headers they want, including authentication ones.
>
> - If you don't use the header authentication for anything in a given service these issues will not be a problem for that service.


## Usage

The CLI uses subcommands (powered by Cobra + Viper).

```bash
# Show all commands and global flags
ts-proxyd --help

# Start the proxy (all servers defined in the config)
ts-proxyd server --config /etc/ts-proxy/config.yaml

# Dry-run: authenticate the Tailscale nodes and print the resolved
# server structure, then exit without serving traffic.
ts-proxyd server --dry-run --config config.yaml

# Print the fully resolved configuration (very useful for debugging
# env expansion, defaults, and validation errors).
ts-proxyd config --config config.yaml
```

Global flags (available to all commands):

- `--config` – path to the YAML config file. If omitted, ts-proxyd looks for
  `ts-proxy.yaml` in the current directory, `$HOME/.config/ts-proxy/`, and
  `/etc/ts-proxy/`.
- `--state-dir` – base directory for Tailscale state (overwrites the value in the config file).
- `--stop-on-fail` – if any server fails, stop the whole process (instead of restarting the failed one).

See `ts-proxyd server --help` for the `--dry-run` flag.

## Configuration

Configuration is done via a YAML file (recommended), combined with a small number of
command-line flags and environment variables (Traefik-style).

See the well-commented [example-config.yaml](example-config.yaml) in the root of this repository for a complete example covering:

- Multiple independent Tailscale nodes ("servers")
- Named auth tokens (1 token can be used by many servers)
- Both HTTP and raw TCP handlers
- TLS termination + Tailscale Funnel on selected handlers
- Environment variable expansion inside `auth_key` values (`${TS_AUTHKEY}` etc.)

A minimal example:

```yaml
state_dir: /var/lib/ts-proxy
stop_on_fail: false

tokens:
  prod:
    auth_key: "${TS_AUTHKEY}"

servers:
  web:
    hostname: my-service
    token: prod
    handlers:
      - type: http
        listen: ":80"
        upstream_address: "127.0.0.1:8080"
      - type: http
        listen: ":443"
        upstream_address: "127.0.0.1:8080"
        tls: true
        funnel: true   # expose publicly via Tailscale Funnel
```

### Important notes about configuration
- Server and token names must match `^[a-zA-Z0-9_]+$` (letters, numbers, underscore).
- Each server gets its own subdirectory under `state_dir/<server-name>`.
- `auth_key` values containing `${VAR}` are expanded at load time using the process environment.
- The `config` subcommand shows you exactly what will be used after defaults are applied and variables expanded.
- You can override `state_dir` and `stop_on_fail` from the command line or `TS_PROXY_*` environment variables.

## Release schedule
Version structure example: 0.7.10
  - 0: major
  - 7: minor
  - 10: patch

Each week an automatic PR is sent to update the package dependencies. For each update there will be a patch release.

Bug fixes would be shipped in patch releases.

Anything that has breaking changes by changing something in this repository will be released as a minor release.

No plans for bumping the major versions yet. 

## Next steps
- [ ] A way to expose a folder, maybe using single page application patterns, instead of only ports.
- [x] Multi-server support via YAML configuration (and a single process) — implemented. See `example-config.yaml` and the `server` / `config` subcommands.

## Related projects
This project re-uses the same Tailscale `tsnet` + header primitives as
[tclip](https://github.com/tailscale-dev/tclip).
