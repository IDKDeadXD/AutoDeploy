# Releasing

Set the version in `internal/cli/cli.go`, update `CHANGELOG.md`, run the test suite, then build the two supported Linux targets:

```bash
mkdir -p dist
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X github.com/idkde/deploy-agent/internal/cli.Commit=$(git rev-parse --short HEAD) -X github.com/idkde/deploy-agent/internal/cli.Built=$(date -u +%FT%TZ)" -o dist/deploy-linux-amd64 ./cmd/deploy
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X github.com/idkde/deploy-agent/internal/cli.Commit=$(git rev-parse --short HEAD) -X github.com/idkde/deploy-agent/internal/cli.Built=$(date -u +%FT%TZ)" -o dist/deploy-linux-arm64 ./cmd/deploy
sha256sum dist/* > dist/checksums.txt
```
