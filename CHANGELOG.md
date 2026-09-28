# Changelog

Una entrada por release, en el orden en que salieron. Se genera del historial, que
ya dice lo que pasó:

```bash
git log --format="- %s" v0.18.0..v0.19.0
```

Las versiones siguen el tipo del commit: `feat:` sube el *minor*, `fix:` y `perf:`
suben el *patch*. Con una excepción histórica que se queda como está — `v0.17.0` es
un `fix:` que se llevó un *minor*, y reescribir un tag publicado rompe a quien ya lo
descargó.

## No publicado

- docs: bring the readme, architecture and roadmap up to adoption and history
- feat: keep a history of what croft did, and say which deployment failed and why
- feat: redeploy a service as it is configured, with no form to fill in
- feat: take on a site the container's own web server serves, and publish it
- feat: take on a service croft found running, and redeploy it from then on

### v0.22.0 — 2026-09-26
- docs: file v0.21.0 and v0.22.0 in the changelog
- fix: forward restart, stop and start from the panel to the agent
- feat: see and bounce the units croft found but did not deploy

### v0.21.0 — 2026-09-25
- feat: restart, stop and start a service without a full deploy
- feat: let a deployed service's branch change without losing its env
- docs: file v0.20.0 where it already shipped

### v0.20.0 — 2026-09-21
- feat: a database inside the container, so one snapshot holds both
- fix: give an older unit the line it needs to read generated credentials
- fix: the health path can no longer reach the shell croft runs as root
- fix: reload nginx the way this host does it, so a vhost lands on Alpine too
- fix: count login attempts by an address the caller cannot choose
- fix: refuse a rollback to a snapshot that is not there, whatever it is called
- fix: wait for a health check that expects a 4xx
- fix: dial every route at once, not one after another
- fix: stop announcing steps before they run, when enabling TLS
- fix: forget finished jobs, so a long-lived daemon stops growing
- fix: build vhost paths as Linux paths, whatever compiled croft
- fix: say out loud when the installer has not started anything
- docs: bring the roadmap up to what is actually built
- build: fall back to the local toolchain when there is no docker daemon
- build: let NO_DOCKER=1 skip a daemon that answers but cannot write

### v0.19.0 — 2026-09-12
- feat: remove a service

### v0.18.2 — 2026-09-12
- perf: read a container in three calls instead of twenty

### v0.18.1 — 2026-09-12
- fix: stop the container page asserting things it does not know

### v0.18.0 — 2026-09-12
- feat: a page for what a container is actually doing
- docs: bring the architecture up to what was built

### v0.17.1 — 2026-09-11
- fix: send the form when the method is not POST

### v0.17.0 — 2026-09-11
- fix: propose a Node that can build current projects

### v0.16.3 — 2026-09-11
- fix: install a Node that can build current projects

### v0.16.2 — 2026-09-11
- fix: say which step is running, rather than ticking it off early

### v0.16.1 — 2026-09-11
- fix: let progress through nginx, and make Cancel mean it

### v0.16.0 — 2026-09-11
- feat: services, environment, readiness and a way back

### v0.15.0 — 2026-09-10
- feat: deploy a project, in two plans

### v0.14.0 — 2026-09-10
- feat: expose the panel from settings and undo a rejected vhost

### v0.13.0 — 2026-09-10
- feat: put the panel on a domain of its own and slow down guessing

### v0.12.1 — 2026-09-10
- fix: keep the separator so a name that looks like a flag survives
- test: cover the plan, the agent boundary and the domain services

### v0.12.0 — 2026-09-10
- feat: edit routes, renew certificates and start or stop containers

### v0.11.1 — 2026-09-10
- fix: fold a step's own narration under it instead of repeating the command

### v0.11.0 — 2026-09-10
- feat: report a route where nothing is listening

### v0.10.1 — 2026-09-10
- fix: build the tls route once so the plan matches what is written

### v0.10.0 — 2026-09-10
- feat: turn on https for a domain from the panel

### v0.9.0 — 2026-09-10
- feat: configure dns credentials from the panel and read certbot's

### v0.8.0 — 2026-09-09
- feat: issue certificates with an embedded acme client

### v0.7.0 — 2026-09-09
- feat: add and remove domains from the panel

### v0.6.1 — 2026-09-09
- fix: restart both halves on update and report a version mismatch clearly

### v0.6.0 — 2026-09-09
- feat: add the certificates section
- feat: read certificates from the host and warn before they expire

### v0.5.1 — 2026-09-09
- fix: print a usable url and refuse a taken username before prompting

### v0.5.0 — 2026-09-09
- docs: describe the two processes
- feat: run the panel as an unprivileged user
- feat: split the privileged half into an agent behind a unix socket

### v0.4.0 — 2026-09-09
- feat: add croft update so upgrading is one command

### v0.3.1 — 2026-09-09
- fix: stamp the version into the binary instead of hardcoding it

### v0.3.0 — 2026-09-09
- docs: add the teams phase and mark what is done
- feat: create an account during unattended installation
- feat: generate a random admin password instead of a default one
- feat: add the plan dialog and container actions
- feat: wire the job runner into the daemon
- feat: run writes as background jobs with streamed progress
- feat: describe every write as a plan the execution walks

### v0.2.0 — 2026-09-09
- style: format the overview query
- docs: document signing in and creating users
- feat: create the first user during installation
- feat: add the login screen and sign out
- fix: parse flags that come after positional arguments
- feat: add users, sessions and a login guard
- fix: replace the binary by rename so upgrades work while it is running

### v0.1.1 — 2026-09-09
- feat: derive container domains from the routes that serve them
- fix: read the effective nginx configuration instead of our own directory
- fix: revert the nginx change when the config is rejected

### v0.1.0 — 2026-09-09
- docs: keep strategy notes out of the repository
- docs: document installing and running the panel
- feat: add installer that provisions dependencies and the daemon
- feat: add web interface
- feat: add http api and cli
- feat: add drift detection between containers and routes
- feat: add route module with nginx driver
- feat: add instance module with LXD and Incus driver
- feat: add host abstraction and id value object
- chore: ignore build artefacts and force LF endings
- docs: update the prototype repository link
- docs: add portability section with runtime and proxy drivers
- docs: add architecture and roadmap
- docs: add readme
- chore: add AGPL-3.0 license
- chore: add gitignore

