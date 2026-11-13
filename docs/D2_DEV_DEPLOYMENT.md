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

## Deployment evidence and limits (2026-11-12)

The initial D2 rollout ran committed release `2fb973d9057957d5ad7570ec22c273226e04f190` from the approved D2 branch. `scripts/test-release.sh` passed during that release: all focused packages, including both ten-hand black-box fixtures, and the root build. The local HTTP health and two-client WebSocket start/reconnect smoke passed. The same two-client smoke ran **on the droplet** against its forwarded `127.0.0.1:18081` and passed, proving the SSH tunnel carries HTTP and WebSocket traffic. After a Trick-only service/tunnel restart, local smoke and droplet health passed again. The game, tunnel, and deploy timer were enabled and active; `Linger=yes`. StarCoaster's game and public tunnel were still active afterward. Read `/healthz` and `state/deployed` for the current release SHA.

The main-only deploy service was triggered without a main update. It fetched `origin/main` at `5b570d549fe5dbcf943e0eb9008cecd9c715ed4f`, logged `unchanged ... no gate run`, and retained the D2 branch release. The normal timer interval is five minutes, not a GitHub webhook. A controlled rollback used the previously built `2c837b0` release with its known failing smoke binary; the deploy command failed, restored `current` to `2fb973d`, restarted it, and `/healthz` again reported `2fb973d`. No second release gate was run for the reused candidate. The initial attempt at `ab25a20` failed in the archived build before switching because Go VCS stamping expected a Git checkout; `2c837b0` fixed the build environment, and `2fb973d` fixed the smoke's initial lobby expectation and explicit failed-health rollback.

**Public HTTPS is live after the DNS correction.** Porkbun had no explicit Trick record: `trick.noremac.dev` inherited the `*.noremac.dev` A record at `146.190.175.159`. The new Pi-held Porkbun key was validated with a read-only API call, then the updater created only an explicit `trick.noremac.dev` A record (`603310829`) at `165.227.55.254`. The wildcard stayed at `146.190.175.159`, and StarCoaster and Nova remained at `165.227.55.254`. All four authoritative Porkbun nameservers and the tested 1.1.1.1, 8.8.8.8, and 9.9.9.9 resolvers returned the new Trick address; no Trick AAAA/CNAME exists. Caddy validated and reloaded its unchanged configuration, then obtained a trusted Let's Encrypt certificate. Normal `https://trick.noremac.dev/healthz` returned the deployed commit from both the droplet and Pi. From the Pi, **two-client WSS start/reconnect** passed first with the new IP pinned during DNS propagation and then on the normal, unpinned public URL with TLS hostname verification enabled. The Pi's Tailscale resolver briefly returned the cached old wildcard answer immediately after the change, then converged.

For future Porkbun DNS changes from the Pi, keep the account's key outside Git at `/home/pi/.config/porkbun/credentials.json` (parent mode `700`, file mode `600`, owned by `pi`). This is a general Porkbun location shared by Pi projects. Its JSON keys are `apikey` and `secretapikey`; never paste either into a shell command, commit, log, or chat. Enable API access for `noremac.dev`. The key has full account access, so keep this file private and rotate the key if the Pi is compromised. `scripts/porkbun-trick-dns.py` reads this file and changes **only the `trick.noremac.dev` A record**. It creates an explicit Trick record only if the hostname currently inherits one unambiguous wildcard A record; otherwise it edits the exact record ID. It refuses duplicate A records, AAAA/CNAME conflicts, and an unexpected current address. Edits preserve the record's TTL and other fields. Use:

```sh
python3 scripts/porkbun-trick-dns.py status
python3 scripts/porkbun-trick-dns.py set NEW_IPV4 --expected-current 165.227.55.254
```

Replace `NEW_IPV4` with the intended new address. The command reads back the record after a write. Check authoritative DNS separately, then rerun the public smoke. To test the new Caddy target while a local resolver still caches the old address, build the checked-in `cmd/deploy-smoke` and use `-connect-ip 165.227.55.254`; it still verifies the URL hostname and TLS certificate. See the [official Porkbun DNS API reference](https://porkbun.com/llms/dns) for endpoint behavior.

No full Pi reboot was performed because it hosts other active services. Enabled units plus `Linger=yes` establish the boot path, and a safe service/tunnel restart passed; an actual reboot remains unverified. The D1 race test remains unverified on this Pi due to its ThreadSanitizer VMA limitation, and the manual Ubuntu CI workflow has not run because this branch has not been pushed. No main push or merge was performed, so later automatic updates will not contain D2 until that happens.
