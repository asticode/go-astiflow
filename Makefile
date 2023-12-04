coverage:
	go test -count=1 github.com/asticode/go-astiflow/... -coverprofile=coverage.out
	go tool cover -html=coverage.out

test:
	go test -count=1 github.com/asticode/go-astiflow/...

dev-go:
	go run -tags dev ./internal/cmd/dev/main.go

dev-web:
	cd pkg/plugins/monitor/server/web && npm run build-watch

push:
	git add --all
	GIT_COMMITTER_DATE="Sun Dec 4 12:22:29 2023 +0100" git commit --date="Sun Dec 4 12:22:29 2023 +0100" --amend
	git push -f