# Deploy Agent

Deploy Agent is a small Linux daemon that receives signed GitHub push webhooks and deploys registered Git repositories. One daemon listens on one port and queues deployments independently for each project.

## Install

Build or download the `deploy` binary, copy it to `/usr/local/bin/deploy`, then run:

```bash
sudo deploy install --user "$SUDO_USER" --listen 127.0.0.1 --port 4747 --public-url https://deploy.example.com
```

`install` runs the daemon as the non-root account that owns your deployment repositories. This preserves that account's SSH credentials and Docker access, so a normal `deploy init` works directly from its existing checkout. It creates protected state directories and the systemd unit, then enables the service. The daemon is deliberately not a TLS terminator. Use a reverse proxy for public HTTPS:

It also copies the executable into that account's `~/.local/bin/deploy`. After installation, updating needs no elevated privileges:

```bash
deploy update
```

The command checks the latest `IDKDeadXD/AutoDeploy` GitHub release, downloads the matching Linux architecture asset, verifies `checksums.txt`, atomically replaces the user-owned binary, and restarts the daemon. Use `deploy update --repo owner/name` to use a fork or another release repository.

```caddy
deploy.example.com {
    reverse_proxy 127.0.0.1:4747
}
```

## Register a repository

From the repository that should be deployed:

```bash
cd ~/flux
deploy init
```

The wizard writes a readable `deploy/config.json` in the repository, registers the project in `/etc/deploy-agent/projects`, and stores secrets outside the repository with owner-only permissions. It prints a unique hook URL and secret once. In GitHub, add a repository webhook using that URL, `application/json`, the displayed secret, and **Push events**.

`hard-reset` is the default update strategy. It fetches the configured remote branch and resets only tracked repository files; it never runs `git clean`. Shell commands in project configuration are intentional administrator-controlled input and run through `/bin/sh -c`; use `program` and `args` command objects when a shell is not needed.

## Operations

```bash
deploy list
deploy status --project flux
deploy run --project flux
deploy run --project flux --dry-run
deploy history --project flux
deploy logs --project flux
deploy doctor --project flux
deploy config validate
deploy config command 'docker compose up -d --build --remove-orphans'
deploy webhook show
deploy webhook reveal --yes
deploy webhook rotate
```

Deployments for one project never overlap. If a deployment is running, a newer webhook replaces its pending job. Projects can run concurrently up to the configured global limit. The public server exposes only `GET /health` and signed `POST /hooks/{project}/{token}`; local control remains on `/run/deploy-agent/deploy.sock`.

## Discord

```bash
deploy notifications discord setup
deploy notifications discord status
deploy notifications discord disable
deploy notifications discord enable
deploy notifications discord remove
```

Discord URLs live in `/etc/deploy-agent/secrets` and are never included in status output. Docker deployments require the account selected at installation to access Docker, usually through the `docker` group. That group is effectively privileged on most hosts; grant it only after reviewing this implication.

## Troubleshooting and removal

Use `deploy doctor`, `systemctl status deploy-agent`, and `journalctl -u deploy-agent` first. Check that GitHub can reach the configured public URL and that the service user can fetch the remote and run the deployment command.

To remove the systemd unit without deleting project state or repositories:

```bash
sudo deploy uninstall
```

Remove state directories manually only after backing up any history you want to retain.

## Development

```bash
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 go build -trimpath -o dist/deploy-linux-amd64 ./cmd/deploy
GOOS=linux GOARCH=arm64 go build -trimpath -o dist/deploy-linux-arm64 ./cmd/deploy
```

See [docs/architecture.md](docs/architecture.md) and [docs/releasing.md](docs/releasing.md).

To publish a release from a development machine with GitHub CLI authenticated, run `npm run update`. It builds both Linux assets, writes checksums, and publishes the next patch release. Use `npm run update -- 0.2.0` to choose the version explicitly; this command is not used on deployment servers.
