# Fiskus — bežné príkazy. Na Windows funguje cez Git Bash alebo WSL.

WEB_DIST := backend/internal/web/dist
VERSION  ?= dev

.PHONY: test test-go test-web web build demo dev-api dev-web

test: test-go test-web

test-go:
	cd backend && go vet ./... && go test -race ./...
	cd backend && go test -tags sqlite ./...

test-web:
	cd frontend && npm test

# zostaví frontend priamo do Go balíka web (vloží sa do binárky)
web:
	cd frontend && npx vite build --outDir ../$(WEB_DIST) --emptyOutDir
	touch $(WEB_DIST)/.gitkeep

build: web
	cd backend && CGO_ENABLED=0 go build -tags sqlite \
		-ldflags "-s -w -X main.version=$(VERSION)" -o ../bin/fiskus ./cmd/fiskus
	cd backend && CGO_ENABLED=0 go build -ldflags "-s -w" -o ../bin/fiskus-mcp ./cmd/fiskus-mcp

# ukážka s demo dátami: http://127.0.0.1:8080
demo: web
	cd backend && go run ./cmd/fiskus -demo

# vývoj: v jednom termináli API, v druhom frontend s okamžitým obnovovaním
dev-api:
	cd backend && go run ./cmd/fiskus -demo

dev-web:
	cd frontend && npm run dev
