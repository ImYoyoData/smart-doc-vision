.PHONY: backend web build test docker-up docker-down

backend:
	cd backend && go run ./cmd/server

web:
	cd web && npm run dev

build:
	cd backend && go build ./...
	cd web && npm run build

test:
	cd backend && go test ./...
	cd web && npm run build

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
