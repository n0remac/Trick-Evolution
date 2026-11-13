# D2 dev Pi deployment

## Topology and release policy

The Pi runs Trick Evolution as the `pi` systemd user on `127.0.0.1:18081`. A separate, restricted `trick-tunnel` SSH account forwards that port to `127.0.0.1:18081` on the Caddy droplet at `165.227.55.254`. Caddy serves `trick.noremac.dev` over HTTPS and proxies WebSocket upgrades. The game remains public at that name: anyone with the URL can create a room. Room seats use private session tokens; room state lives only in process memory and is lost on restart.

`/home/pi/services/trick-evolution` contains `source.git`, `releases/<40-character-commit>`, the atomic `current` symlink, `state/{deployed,last-main-seen,failed-main}`, `logs/deploy.log`, and the installed deploy script under `bin`. The running service never executes from the development checkout. Releases are read-only. The deploy log and systemd journal contain build, gate, and health results; no room tokens are printed.

The timer invokes `deploy-dev-pi.sh --main` every five minutes after boot. This mode fetches **only `origin/main`**, checks that it advances from the recorded main baseline, and deploys a new SHA once. A failed SHA is recorded and skipped by later timer ticks, avoiding repeated expensive gates. To retry that same main SHA after fixing an environmental problem, run `deploy-dev-pi.sh --main --retry` manually. `--from-local <full SHA>` is only for an explicitly approved initial rollout from this D2 branch. It records the current `origin/main` baseline so the unchanged, older main cannot replace that test release. **Future automatic updates require pushing or merging the D2 code to GitHub main; this rollout does not do that.** A non-fast-forward rewrite of main is refused.

For each new SHA, the deploy script archives committed source into an isolated staging directory, runs `scripts/test-release.sh` once (focused Go, ten-hand HTTP/WebSocket fixtures, root build), builds the server with its commit embedded and a two-seat smoke executable, installs a read-only release, switches `current` atomically, and restarts the service. It checks `/healthz` for the exact commit and joins, starts, and reconnects two WebSocket clients. A failed restart/health check restores the previous symlink and restarts the previous release. The optional full Chromium and race gates remain separately invokable as documented in [D1_RELEASE_GATES.md](D1_RELEASE_GATES.md).

## Repeatable operator commands

From this repository on the Pi, install or update the versioned user units and deploy script:

```sh
scripts/install-dev-pi-units.sh
systemctl --user daemon-reload
```

For an approved first branch rollout, commit the tree, then run:

```sh
/home/pi/services/trick-evolution/bin/deploy-dev-pi.sh --from-local "$(git rev-parse HEAD)"
systemctl --user enable --now trick-evolution.service trick-evolution-tunnel.service trick-evolution-deploy.timer
```

The tunnel needs `/home/pi/.ssh/trick_evolution_proxy_ed25519` (mode 600), the remote `trick-tunnel` account with an authorized key limited to `127.0.0.1:18081`, and `/etc/ssh/sshd_config.d/62-trick-tunnel.conf` on the droplet. The versioned restriction is in `deploy/remote/trick-tunnel.sshd.conf`; validate with `sshd -t` before reloading SSH. The Caddy route is `deploy/caddy/trick.caddy`; `scripts/install-caddy-route.sh` stages it over `caddy-admin`, backs up an existing Trick route, validates the entire Caddyfile, and reloads Caddy. On validation or reload failure it restores the prior Trick route and reloads again. It does not edit other site files.

```sh
systemctl --user status trick-evolution.service trick-evolution-tunnel.service trick-evolution-deploy.timer
journalctl --user -u trick-evolution.service -u trick-evolution-tunnel.service -n 80 --no-pager
journalctl --user -u trick-evolution-deploy.service -n 80 --no-pager
tail -n 80 /home/pi/services/trick-evolution/logs/deploy.log
cat /home/pi/services/trick-evolution/state/deployed
curl -fsS http://127.0.0.1:18081/healthz
/home/pi/services/trick-evolution/current/bin/deploy-smoke -url https://trick.noremac.dev -commit "$(cat /home/pi/services/trick-evolution/state/deployed)"
```

To roll back to a previously tested release, atomically repoint `current` to `releases/<SHA>`, restart `trick-evolution.service`, and run that release's smoke binary with `-commit <SHA>`. The deploy script does this automatically on failed health. A manual rollback does not alter the main baseline; if main has a new SHA, the timer can deploy it on its next tick. Stop the deploy timer while investigating a production issue. Use `systemctl --user restart trick-evolution.service` for a routine process restart; active rooms disappear. `loginctl show-user pi -p Linger` confirms boot persistence (`Linger=yes` is required); the Pi's other services make a full reboot a coordinated operation.

## Deployment evidence and limits

Deployment evidence is recorded after first rollout below. DNS must resolve `trick.noremac.dev` to the Caddy droplet for the normal public URL to work. A direct `curl --resolve trick.noremac.dev:443:165.227.55.254 https://trick.noremac.dev/healthz` can test Caddy before DNS is corrected; it does not prove the public DNS path.
