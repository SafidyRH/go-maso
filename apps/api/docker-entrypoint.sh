#!/bin/sh
set -eu

/usr/local/bin/go-maso-migrate up
exec /usr/local/bin/go-maso-api
