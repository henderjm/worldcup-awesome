#! /usr/bin/env bash

GOOS=linux GOARCH=amd64 go build -o hackday orchestrator.go

docker build -t henderjm/wcawesome-orchestrator .
