# Repository Guidelines

## Project Structure & Module Organization
`main.go` is the CLI entrypoint that subscribes to Redis Sentinel events and dispatches DNS updates. Shared config parsing lives in `config/`, with `config/demo-config.yaml` as the reference template. Provider abstractions are in `provider/provider.go`; each concrete DNS backend gets its own subdirectory such as `provider/huaweicloud-private/` or `provider/tencentcloud-private/`. Build artifacts are written to `bin/` and should not be edited by hand.

## Build, Test, and Development Commands
Use Go 1.27 to match `go.mod`.

- `go build -o sentinel-sync-dns` builds the local binary for the current platform.
- `go test ./...` runs all package tests; today it mainly verifies that packages compile.
- `go test -cover ./...` checks coverage before submitting behavior changes.
- `./build.sh` creates Linux and Windows binaries, zips them into `bin/sentinel-sync-dns.zip`, and removes temporary executables afterward.

When validating locally, run the binary with a sanitized config, for example: `./sentinel-sync-dns -config=config/demo-config.yaml`.

## Coding Style & Naming Conventions
Follow standard Go formatting: tabs for indentation, `gofmt` formatting, and package-focused file layout. Keep package names lowercase; provider package directories use hyphenated folder names while Go package identifiers stay underscore-based, for example `provider/tencentcloud-private` -> `package tencentcloud_private`. Exported names use `CamelCase`; YAML field names must stay aligned with existing keys such as `dns-providers`, `zone_id`, and `secretId`.

## Testing Guidelines
There are currently no `*_test.go` files, so new logic should add focused table-driven tests beside the package under test. Prioritize config loading, provider factory validation, and provider-specific request shaping. For cloud integrations, prefer mocks or thin interface seams over live credentials. Treat `go test ./...` as the minimum gate and add coverage for every new branch you introduce.

## Commit & Pull Request Guidelines
Recent history uses short imperative subjects such as `support tencent cloud (#1)` and `first provider`. Keep commits concise, descriptive, and scoped to one change; avoid placeholders like `test`. PRs should explain the operational impact, list config changes, and include the exact verification command output. If a change touches provider behavior, include a sanitized config example or relevant logs instead of screenshots.

## Security & Configuration Tips
Never commit real Sentinel passwords, cloud keys, or zone IDs. Copy `config/demo-config.yaml` for local testing and keep secrets outside version control. Review `bin/` before committing to avoid shipping stale release artifacts unintentionally.
