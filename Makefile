.PHONY: dev run up down build clean help

ENV_FILE ?= .env

help:
	@echo "make dev    - run locally (requires Go + qpdf)"
	@echo "make up     - build & run via docker compose"
	@echo "make down   - stop docker compose"
	@echo "make build  - compile binary"
	@echo "make clean  - remove binary and out/"

dev:
	@set -a; . ./$(ENV_FILE); set +a; go run .

run: dev

up:
	docker compose up --build --remove-orphans

down:
	docker compose down

build:
	go build -o paychecks .

clean:
	rm -rf out/ paychecks
