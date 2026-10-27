
# The web UI's browser dependencies come from npm (web/ui/package.json).
# node_modules there is committed, exactly as npm ci leaves it, so the Go
# module embeds it and `go get` delivers it. After changing package.json,
# run `make deps` and commit the result; check-deps fails when the
# committed files differ from the lockfile.
.PHONY: deps check-deps
deps:
	npm install --prefix web/ui --no-audit --no-fund

check-deps:
	npm ci --prefix web/ui --no-audit --no-fund
	git diff --exit-code --stat -- web/ui/node_modules
	@test -z "$$(git status --porcelain -- web/ui/node_modules)" || (git status --short -- web/ui/node_modules; exit 1)
