# Releasing

From your development machine, after committing the release-ready source and authenticating `gh`:

```bash
npm run update
```

This is the normal release path. It requires a clean Git worktree, pushes the current commit to `origin`, increments the latest GitHub release's patch version, builds `linux-amd64` and `linux-arm64`, generates checksums, and creates the GitHub Release. Use `npm run update -- 0.2.0` to publish an explicit version. The VPS never runs this command; it only runs `deploy update`.

Manual release steps:

Set the version in `internal/cli/cli.go`, update `CHANGELOG.md`, run the test suite, then build the two supported Linux targets:

```bash
mkdir -p dist
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X github.com/idkde/deploy-agent/internal/cli.Commit=$(git rev-parse --short HEAD) -X github.com/idkde/deploy-agent/internal/cli.Built=$(date -u +%FT%TZ)" -o dist/deploy-linux-amd64 ./cmd/deploy
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X github.com/idkde/deploy-agent/internal/cli.Commit=$(git rev-parse --short HEAD) -X github.com/idkde/deploy-agent/internal/cli.Built=$(date -u +%FT%TZ)" -o dist/deploy-linux-arm64 ./cmd/deploy
sha256sum dist/* > dist/checksums.txt
```
