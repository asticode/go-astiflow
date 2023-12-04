example=demuxing_decoding
args=-i ./testdata/video.mp4
mode=valgrind
lib=astiav

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
	GIT_COMMITTER_DATE="Sun Dec 4 12:22:29 2023 +0100" git commit -a --date="Sun Dec 4 12:22:29 2023 +0100" --amend
	git push -f

testleak-docker-build:
	docker build -t astiflow-testleak ./internal/testleak

testleak-docker-binary-build:
	mkdir -p ./internal/testleak/tmp/gocache
	mkdir -p ./internal/testleak/tmp/gomodcache
	docker run -v ${CURDIR}/..:/opt/asticode astiflow-testleak /opt/go/bin/go build -o ./internal/testleak/tmp/app-docker ./examples/${example}/main.go

testleak-docker-binary-run:
	docker run -v ${CURDIR}/..:/opt/asticode -e TESTLEAK_MODE=${mode} -e TESTLEAK_ARGS="${args}" astiflow-testleak

examples-docker-build:
	docker build -t astiflow-examples ./examples

examples-run:
	mkdir -p ./examples/tmp/gocache
	mkdir -p ./examples/tmp/gomodcache
# We can't use "go run" directly as it doesn't seem to be catching/forwarding sigint
	docker run -v ${CURDIR}/..:/opt/asticode astiflow-examples /opt/go/bin/go build -o ./examples/tmp/${lib}-${example} ./examples/${lib}/${example}/main.go
	docker run -v ${CURDIR}/..:/opt/asticode -p 4000:4000 astiflow-examples ./examples/tmp/${lib}-${example} ${args}