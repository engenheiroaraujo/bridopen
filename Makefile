# Requires GNU Make. On Windows, prefer the GNU Make installed by Chocolatey.
#$env:Path = "C:\ProgramData\chocolatey\bin;" + $env:Path

POSTGRESQL_URL := $(if $(strip $(BRIDOPEN_DB_CONN_URL)),$(BRIDOPEN_DB_CONN_URL),postgres://postgres:postgres@127.0.0.1:5436/postgres?sslmode=disable)
COMPOSE_PROD := docker compose -f docker-compose.yml
COMPOSE_LOCAL := docker compose -f docker-compose.local.yml
COMPOSE_DEV := docker compose -f docker-compose.dev.yml

build-up:
	go build ./cmd/http/.

run:
	go run ./cmd/http/.

server: run

exp:
# 	go run ./cmd/exp/connect-database-exp.go
	go run ./cmd/exp/exp.go

database:
	$(COMPOSE_LOCAL) up -d

# testes Go (inclui a verificação dos scripts dos templates base)
test:
	go test -count=1 ./...

# teste unitário do frontend em Node, sem navegador
test-frontend:
	npm run test:frontend

# testes em navegador real; o Playwright sobe o cmd/e2e-preview sozinho
test-e2e:
	npm run test:e2e

# instala dependências Node e o Chromium do Playwright (uma vez por máquina)
test-e2e-install:
	npm ci
	npx playwright install chromium

# tudo: Go, Node e navegador
test-all: test test-frontend test-e2e

# cria arquivo de migration
migrate-create:
ifndef name
	$(error Informe o nome da migration. Exemplo: make migrate-create name=create_users)
endif
	migrate create -ext sql -dir db/migrations -seq $(name)

# executa migrations locais
ifneq ($(strip $(POSTGRESQL_URL)),)
migrate-up:
	migrate -database "$(POSTGRESQL_URL)" -path db/migrations up
else
migrate-up:
	$(error DB_CONN_URL is not set. In PowerShell: $$env:DB_CONN_URL="postgres://..." before running make)

endif

# dropa migrations locais
ifneq ($(strip $(POSTGRESQL_URL)),)
migrate-down:
	migrate -database "$(POSTGRESQL_URL)" -path db/migrations down
else
migrate-down:
	$(error DB_CONN_URL is not set. In PowerShell: $$env:DB_CONN_URL="postgres://..." before running make)

endif

prod-env:
	chmod 600 .env

prod-config: prod-env
	$(COMPOSE_PROD) config --quiet

prod-db: prod-config
	$(COMPOSE_PROD) up -d db

prod-migrate: prod-db
	$(COMPOSE_PROD) run --rm migrate

prod-up: prod-migrate
	$(COMPOSE_PROD) up -d --build --remove-orphans server caddy

# prepara .env, sobe db, roda migrations, rebuilda server/caddy
prod-deploy: prod-up
	docker image prune -f

# status dos containers
prod-ps:
	$(COMPOSE_PROD) ps

# logs do server
prod-logs:
	$(COMPOSE_PROD) logs -f server

# logs do caddy
prod-caddy-logs:
	$(COMPOSE_PROD) logs -f caddy

# logs do banco
prod-db-logs:
	$(COMPOSE_PROD) logs -f db

# derruba containers sem apagar volume
prod-down:
	$(COMPOSE_PROD) down --remove-orphans

# derruba e apaga volumes, cuidado: apaga banco
prod-reset:
	$(COMPOSE_PROD) down -v --remove-orphans

.PHONY: build-up server exp database test test-frontend test-e2e test-e2e-install test-all migrate-create migrate-up migrate-down prod-env prod-config prod-db prod-migrate prod-up prod-deploy prod-ps prod-logs prod-caddy-logs prod-db-logs prod-down prod-reset
