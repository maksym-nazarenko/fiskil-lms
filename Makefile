help:
	@sed -n "/^[a-zA-Z0-9_-]*:/ s/:.*//p" < Makefile | sort

test:
	@go test -race ./...

run:
	docker compose -p lms -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up -d
logs:
	docker compose -p lms -f docker/docker-compose.yml -f docker/docker-compose.dev.yml logs -f

down:
	docker compose -p lms -f docker/docker-compose.yml -f docker/docker-compose.dev.yml stop

clean:
	docker compose -p lms -f docker/docker-compose.yml -f docker/docker-compose.dev.yml rm -fs
