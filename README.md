# Log monitoring server
A log monitoring solution for Fiskil interview


## Requirements
* Docker engine
* GNU make

## Configuration

Configuration is done via environment variables.

|Name|Type|Example|Description|
|-|-|-|-|
|LMS_STOP_AFTER|time duration|10s, 1m, 500ms|Stop execution after this period (used to auto stop e2e test)
|LMS_FLUSH_INTERVAL|time duration|10s, 1m, 500ms|Fixed time period at which DataCollector flushes its message buffer to persistent storage
|LMS_FLUSH_SIZE|integer|100|The same as LMS_FLUSH_INTERVAL but based on number of messages in internal buffer|
|LMS_DB_NAME|string|myDB|Database name to use|
|LMS_DB_USER|string|writeuser|User to access database|
|LMS_DB_PASSWORD|string|secretpass|Password to access database|
|LMS_DB_ADDRESS|string|127.0.0.1:13306, server.example.com|Address of remote database server with or without port information|


## Running project locally

To manipulate local environment, `make` command is being used.

You can run `make help` to see all available targets and short summary about each.
```sh
$ make help
clean - Remove project containers
down - Stop all project containers
logs - Follow logs from all containers in the project
mysql-enter - Run mysql client inside database container
run - Start project in background
test - Run short, non-integrational, tests
test-e2e - Run full end-to-end tests
test-integration - Run all tests, including integration
```

If you prefer running/debugging code from host, you have to run at least database container using:
```sh
$ make run
```
It will run MySQL bound to `127.0.0.1:13306` by default (see [docker-compose.dev.yml](./docker/docker-compose.dev.yml))

## Known issues and trade-offs

1. The storage layer could be abstracted better. I assume, changing MySQL to something else will require refactoring.
1. `make clean` must be run before e2e tests manually
