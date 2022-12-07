#!/bin/sh

# Load up any secrets from the init container
if [ -f /vault/.init-env ]; then
	source /vault/.init-env
fi

# Resolve the Syslog proxies to an IP
export SYSLOG_ADDRESS="`dig $LOGHOST | grep 'IN A' | awk '{print $5}' | head -1`:514"

/logtailer
