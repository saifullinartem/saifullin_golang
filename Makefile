.PHONY: run generate migrate migrate-down test

run:
	@set -e; set -a; . ./.env.example; if [ -f .env ]; then . ./.env; fi; set +a; go run ./cmd/trip-service

generate:
	go tool oapi-codegen -generate types,chi-server -include-operation-ids createTrip,getTrip,finishTrip,health,ready -package api -o api/trip-service.gen.go contracts/openapi/trip-service.openapi.yaml
	sed '/^[[:space:]]*\/\//d' api/trip-service.gen.go > api/trip-service.gen.go.tmp
	mv api/trip-service.gen.go.tmp api/trip-service.gen.go
	gofmt -w api/trip-service.gen.go

test:
	go test -race ./...

migrate:
	@set -e; if [ -f .env ]; then set -a; . ./.env; set +a; fi; test -n "$$DATABASE_URL" || { echo 'DATABASE_URL is required'; exit 1; }; go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@set -e; if [ -f .env ]; then set -a; . ./.env; set +a; fi; test -n "$$DATABASE_URL" || { echo 'DATABASE_URL is required'; exit 1; }; go tool goose -dir migrations postgres "$$DATABASE_URL" down
