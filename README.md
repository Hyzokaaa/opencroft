# OpenCroft

**A PaaS without Docker, that doesn't hijack your server.**

OpenCroft manages LXC system containers, domains and TLS certificates on a single server.
Every container is a full OS — its own init, cron, systemd and filesystem — not a packaged
process.

> **Status: design.** No implementation yet. The primitives were prototyped as shell
> scripts in [croft-tools](https://github.com/Hyzokaaa/croft-tools); this repository
> holds the design and will hold the Go implementation. See [ROADMAP.md](./ROADMAP.md).

## Why

Every tool in this space assumes Docker, and assumes its own database is the source of
truth. Touch the system underneath and the tool drifts out of sync — or quietly overwrites
your changes.

OpenCroft inverts that:

- **The operating system is the source of truth.** LXD knows which containers exist. nginx
  knows which routes exist. certbot knows which certificates exist. OpenCroft reads and writes
  that state, but never owns it.
- **Manual work is a first-class path.** Edit an nginx vhost by hand and OpenCroft detects it,
  shows you the diff, and stops managing that file. It won't fight you.
- **Nothing is hidden.** Every operation can print the exact commands it runs. Use
  `--explain` and learn your system instead of depending on a panel.

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

A single static binary — `croft` — is both the CLI and the daemon, with the web UI
embedded. It runs on the server it manages. Requirements on the host: LXD, nginx, and
certbot for TLS.

```bash
croft create app1 --os ubuntu:24.04 --domain app.example.com
croft list
croft explain
```

## Documentation

| Document | Contents |
|---|---|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Layered design, reconciliation engine, security model |
| [ROADMAP.md](./ROADMAP.md) | Build plan, phase by phase |
| [POSITIONING.md](./POSITIONING.md) | What OpenCroft is, who it is for, how it is described |
| [MONETIZATION.md](./MONETIZATION.md) | Sustainability model |

## Not in scope

- Cluster orchestration — use Kubernetes
- Virtualization management — use Proxmox
- Any operation that cannot be explained as commands you could have typed yourself

## License

[AGPL-3.0](./LICENSE). Free software, with no features held back behind a paywall.
