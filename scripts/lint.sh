#!/bin/bash

go tool github.com/golangci/golangci-lint/cmd/golangci-lint run --fix --out-format=colored-line-number ./...
