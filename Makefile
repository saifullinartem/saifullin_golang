.PHONY: run generate migrate migrate-down test test-integration lint test-mutation

run:
	@set -e; set -a; . ./.env.example; if [ -f .env ]; then . ./.env; fi; set +a; go run ./cmd/trip-service

generate:
	go generate ./internal/... ./cmd/...
	go tool oapi-codegen -generate types,chi-server -include-operation-ids createTrip,getTrip,finishTrip,health,ready -package api -o api/trip-service.gen.go contracts/openapi/trip-service.openapi.yaml
	gofmt -w api/trip-service.gen.go

test:
	go test -race ./...
	go test -race -coverprofile=coverage.out ./internal/domain/... ./internal/usecase/...
	go tool cover -func=coverage.out

migrate:
	@set -e; if [ -f .env ]; then set -a; . ./.env; set +a; fi; test -n "$$DATABASE_URL" || { echo 'DATABASE_URL is required'; exit 1; }; go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@set -e; if [ -f .env ]; then set -a; . ./.env; set +a; fi; test -n "$$DATABASE_URL" || { echo 'DATABASE_URL is required'; exit 1; }; go tool goose -dir migrations postgres "$$DATABASE_URL" down

lint:
	golangci-lint run

test-integration:
	@set -e; set -a; . ./.env.example; if [ -f .env ]; then . ./.env; fi; set +a; go test -race -tags=integration -count=2 ./internal/repository/...

GREMLINS_VERSION := v0.6.0
MUTATION_RUN ?= current

bin/gremlins:
	mkdir -p bin
	GOTOOLCHAIN=go1.26.0 GOBIN="$(CURDIR)/bin" go install github.com/go-gremlins/gremlins/cmd/gremlins@$(GREMLINS_VERSION)

test-mutation: bin/gremlins
	mkdir -p docs/mutation
	GOFLAGS=-race GOTOOLCHAIN=go1.26.0 ./bin/gremlins unleash internal/domain --threshold-efficacy=100 --threshold-mcover=100 --output docs/mutation/$(MUTATION_RUN)-domain.json > docs/mutation/$(MUTATION_RUN)-domain.txt 2>&1
	cat docs/mutation/$(MUTATION_RUN)-domain.txt
	GOFLAGS=-race GOTOOLCHAIN=go1.26.0 ./bin/gremlins unleash internal/usecase --threshold-efficacy=100 --threshold-mcover=100 --output docs/mutation/$(MUTATION_RUN)-usecase.json > docs/mutation/$(MUTATION_RUN)-usecase.txt 2>&1
	cat docs/mutation/$(MUTATION_RUN)-usecase.txt
	python3 scripts/test_mutation.py --phase after --output docs/mutation/$(MUTATION_RUN)-supplemental.json > docs/mutation/$(MUTATION_RUN)-supplemental.txt 2>&1
	cat docs/mutation/$(MUTATION_RUN)-supplemental.txt
	python3 scripts/test_mutation.py --phase after --summary-prefix docs/mutation/$(MUTATION_RUN) --output docs/mutation/$(MUTATION_RUN)-summary.json
