# Fleet Safety Rules

## 1. Response & Code Standards
- Response Style: Direct to the point, no summaries or preamble.
- Code Style: Strict `gofmt`, follow existing patterns, no added docstrings, comments, or type annotations.
- Git Commits: Always author commits as `Mitch <mitch@hairglasses.studio>`.

## 2. Non-Interactive Execution
- Disable terminal pagination (`git --no-pager`) and redirect stdin from `/dev/null` on commands that might await input (`go test ./... < /dev/null`).

## 3. Destructive Mutation Guards
- Never execute unmanaged destructive operations (`rm -rf /`, `git reset --hard`, force pushes).
- Always verify changes via local test suites (`make check-dual`, `make ci`, `go test ./...`) before committing.
