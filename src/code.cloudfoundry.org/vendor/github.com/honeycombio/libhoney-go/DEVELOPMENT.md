# Local Development

## Requirements

Go: https://go.dev/doc/install

## Install Dependencies

```shell
go mod download
```

## Run Tests

To run all tests:

```shell
go test -race -v ./...
```

To run individual tests:

```shell
go test -race -v -run TestEmptyHoneycombTransmission ./...
```
