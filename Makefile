init-pre-commit:
	pip install pre-commit
	pre-commit clean
	pre-commit install --install-hooks --overwrite

	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/mgechev/revive@latest
	go install mvdan.cc/gofumpt@latest
	go install github.com/sqs/goreturns@latest
	go install github.com/go-critic/go-critic/cmd/gocritic@latest
	go install golang.org/x/lint/golint@latest

check:
	pre-commit run --all-files
