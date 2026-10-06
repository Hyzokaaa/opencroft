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

### v0.31.0 — 2026-10-06
- feat: a page for each service with its domains, and every category a list whose rows open what they name
- feat: give a domain to a service rather than to a vhost, keep a route's other names when its own is removed, and mark the last event of a job
- feat: check a domain reaches this server before asking for its certificate, and wait for its DNS instead of failing
- feat: list, add, retry and remove a domain's extra names from the panel
- feat: give a domain more names, each with its own certificate, and put a vhost back as it was when nginx refuses its rewrite
- feat: a Delete action on each snapshot, asking for the container's name where it is the only way back
- feat: delete a snapshot by hand, told first what it was the only way back to

### v0.30.0 — 2026-10-05
- docs: file v0.30.0 in the changelog
- feat: offer the databases found running in a container, and release an adopted one without dropping it
- feat: take on a database already running in a container, without touching it

### v0.29.0 — 2026-10-04
- docs: file v0.29.0 in the changelog
- feat: answer health with the agent's state, so a monitor learns when croft can do nothing

### v0.28.1 — 2026-10-04
- docs: file v0.28.1 in the changelog
- fix: keep the sidebar the height of the window, so its footer is always in sight

### v0.28.0 — 2026-10-04
- docs: file v0.28.0 in the changelog
- docs: describe how the panel lives through its agent being down
- feat: say plainly when the agent is down, recover when it is back, and show where the server is
- fix: keep talking to the agent when it is down or late, and say so instead of failing with whatever the attempt produced

### v0.27.1 — 2026-10-04
- docs: file v0.27.1 in the changelog
- fix: wait for the agent's socket when the panel starts, instead of running without it

### v0.27.0 — 2026-10-04
- docs: file v0.27.0 in the changelog
- docs: describe the panel's shape, saving without deploying, and the demo agent
- fix: keep green for what runs, not for a plan that finished
- fix: say what failed instead of reading for ever, guard the environment editor, and offer only what can work
- fix: let a tap on a phone reach the row menu, and answer the discard question with Escape
- fix: keep unsaved work on every step of a flow, apply saved properties from Properties, and the third review's details
- feat: certificates under domains, a next step after each write, and fixes from the second review
- feat: save a service's properties without deploying it, and show what is waiting for the next deployment
- feat: give the panel a shape — projects as home, addresses, remedies that work, dialogs that go back
- feat: create a container straight into a declared project
- feat: try the panel locally against a demo agent that runs nothing

### v0.26.1 — 2026-10-02
- docs: file v0.26.1 in the changelog
- fix: show dashes as dashes, let a form go with a box unticked or an optional field empty, and use generic examples

### v0.26.0 — 2026-09-30
- docs: file v0.26.0 in the changelog
- docs: describe internal names and a database shared within a project
- feat: connect a container to a database in another container of its project
- feat: show each container's name on the bridge, and offer it where a .env holds an address
- feat: group containers into projects, declared on the host and labelled on each container
- docs: describe projects, declared on the host and labelled on containers
- fix: say when https is served with a wildcard croft already keeps

### v0.25.0 — 2026-09-29
- docs: file v0.25.0 in the changelog
- docs: describe moving a certificate off certbot and wildcards
- feat: let croft own a domain's certificate, from certbot's or as one wildcard for every subdomain
- fix: offer the container a domain already goes to when adding a path

### v0.24.1 — 2026-09-29
- docs: file v0.24.1 in the changelog
- fix: answer the certificate challenge instead of redirecting it
- fix: keep a new variable's row and show the text view plainly

### v0.24.0 — 2026-09-29
- docs: file v0.24.0 in the changelog
- docs: describe paths, websockets and taking over a vhost
- feat: take over a domain whose vhost was written by hand
- feat: send a path of a domain somewhere of its own, with websockets passed through
- docs: describe the environment file and the container's turn
- feat: edit a service's environment in the file it already lives in
- fix: wait for the container instead of colliding with another job on it

### v0.23.0 — 2026-09-27
- docs: file v0.23.0 in the changelog
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

