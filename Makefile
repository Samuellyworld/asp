.PHONY: lint-go test-go

lint-go:
	./scripts/lint_go_ci.sh

test-go:
	cd go-bot && go test ./...
