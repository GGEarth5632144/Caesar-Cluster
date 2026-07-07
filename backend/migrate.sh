#!/bin/bash
export $(grep -v '^#' .env | xargs)
migrate -path migrations -database "$DB_URL" "$@"