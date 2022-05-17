help:
	@sed -n "/^[a-zA-Z0-9_-]*:/ s/:.*//p" < Makefile | sort

test:
	@go test -race ./...

run:

logs:

down:

clean:
