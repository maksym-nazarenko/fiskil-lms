
compose = docker compose -p lms -f docker/docker-compose.yml -f docker/docker-compose.dev.yml

help:
	@sed -n "/^[a-zA-Z0-9_-]*:/ s/:.*//p" < Makefile | sort

test:
	@go test -race ./...

run:
	${compose} up -d

mysql-enter:
	${compose} exec database mysql -uroot -proot

logs:
	${compose} logs -f

down:
	${compose} stop

clean:
	${compose} rm -fs
