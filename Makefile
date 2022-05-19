
compose = docker compose -p lms -f docker/docker-compose.yml -f docker/docker-compose.dev.yml

help:
	@sed -n "/^[a-zA-Z0-9_-]*:/ s/:.*#/ -/p" < Makefile | sort

test: # Run short, non-integrational, tests
	@go test -v -race -short ./...

test-e2e: # Run full end-to-end tests
	@go test -v -race ./...

run: # Start project in background
	${compose} up -d

mysql-enter: # Run mysql client inside database container
	${compose} exec database mysql -uroot -proot

logs: # Follow logs from all containers in the project
	${compose} logs -f

down: # Stop all project containers
	${compose} stop

clean: # Remove project containers
	${compose} rm -fs
