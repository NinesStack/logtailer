#!/bin/sh

# Load up any secrets from the init container
if [ -f /vault/.init-env ]; then
	source /vault/.init-env
fi

# Resolve the Syslog proxies to an IP
export SYSLOG_ADDRESS="`dig $LOGHOST +short | grep -v '\.$' | shuf -n 1`:514"

/logtailer
