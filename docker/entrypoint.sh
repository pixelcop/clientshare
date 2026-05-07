#!/bin/sh

set -e

cd /app

# Run database migrations
./goose -dir ./migrations up

# Start the server
exec ./clientshare
