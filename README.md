# OpenCroft

**A PaaS without Docker, that doesn't hijack your server.**

OpenCroft manages LXC system containers, domains and TLS certificates on a single server.
Every container is a full OS — its own init, cron, systemd and filesystem — not a packaged
process.

> **Status: early but usable.** Containers, domains and TLS certificates are managed from
> the panel, behind a login, with the plan shown before anything runs. Certificates renew
> themselves. Snapshots, logs and a console are not built yet. See [ROADMAP.md](./ROADMAP.md).

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/Hyzokaaa/opencroft/main/install.sh -o install.sh
sudo bash install.sh
```

Detects the distribution and installs what is missing — Incus (or LXD via snap on older
Ubuntu), nginx, the `croft` binary and a systemd unit. `--dry-run` prints the exact
commands and exits without touching anything.

It listens on `127.0.0.1:8080`. Reach it over an SSH tunnel:

```bash
ssh -L 8080:127.0.0.1:8080 you@your-server
```

Or give it a domain of its own and stop tunnelling:

```bash
sudo croft expose panel.example.com
```

That obtains a certificate and writes a vhost — the same path a user's own domain
takes, because a path only the panel uses is a path that rots. It keeps listening on
localhost; nginx is what the internet reaches, and it terminates TLS.

Accounts are created on the host — the panel has no sign-up:

```bash
sudo croft user add <name>
```

Updating later is one command:

```bash
sudo croft update
```

It checks the latest release, verifies the download against its published checksum,
replaces the binary by renaming it into place, and restarts the service. `--check` reports
without touching anything.

To see the panel without a server at all:

```bash
croft serve --demo
```

## Two processes

The half that serves HTTP to a browser runs as an ordinary user and holds no privileges.
A second process, `croft agent`, runs as root and is the only thing that talks to the
container runtime and nginx. They meet over a unix socket owned by the `croft` group.

The agent exposes a closed set of typed operations. There is deliberately no "run this
command" endpoint: the API describes *what* it wants, and the agent decides which commands
that means, validating every field again on its side. A compromised panel cannot ask for
anything the agent was not already willing to do.

## Why

Every tool in this space assumes Docker, and assumes its own database is the source of
truth. Touch the system underneath and the tool drifts out of sync — or quietly overwrites
your changes.

OpenCroft inverts that:

- **The operating system is the source of truth.** LXD knows which containers exist. nginx
  knows which routes exist. The certificate files say when they expire. OpenCroft reads and writes
  that state, but never owns it.
- **Manual work is a first-class path.** Edit an nginx vhost by hand and OpenCroft detects it,
  shows you the diff, and stops managing that file. It won't fight you.
- **Nothing is hidden.** Every write shows the exact commands first, and the panel has a
  mode that prints the command behind everything on screen. Learn your system instead of
  depending on a panel.

```bash
rm /var/lib/croft/croft.db && systemctl restart croft
# everything is still there
```

State that matters lives on the resources themselves, as LXD user annotations. Delete
OpenCroft's database and it rediscovers your infrastructure on the next start.

## How it works

```
Internet (all traffic hits the host IP on port 443)
    |
    nginx (host) — reads the domain, routes to a container
    |
    +-- example.com     --> container-a (10.x.x.2)
    +-- app.example.org --> container-b (10.x.x.3)
    +-- ...             --> container-n (10.x.x.N)
```

A single static binary — `croft` — is the CLI, the daemon and the privileged agent, with
the web UI embedded. Requirements on the host: LXD or Incus, and nginx. ACME is built in,
so certbot is not needed.

```bash
croft list
croft create app1 --port 3000
croft cert issue app.example.com
croft dns show
```

## Documentation

| Document | Contents |
|---|---|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Layered design, reconciliation engine, security model |
| [ROADMAP.md](./ROADMAP.md) | Build plan, phase by phase |

## Tests

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.25-alpine go test ./...
```

`build.sh` runs them before it builds, so a release cannot ship with a failing
test.

They do not need a server, LXD or root. `host.Fake` records the commands a
driver would run instead of running them, which is what makes the interesting
assertions possible: that **the plan shown is the work performed**, that the
address is pinned before the container boots, and that a field the agent should
refuse never reaches a command line.

## Not in scope

- Cluster orchestration — use Kubernetes
- Virtualization management — use Proxmox
- Any operation that cannot be explained as commands you could have typed yourself

## License

[AGPL-3.0](./LICENSE). Free software, with no features held back behind a paywall.
