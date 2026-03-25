#!/bin/bash
# test_uds.sh - Test Alloy using Unix Domain Sockets

# just build-all

SOCK="./alloy.sock"
rm -f "$SOCK"

CORE="./build/dist/usr/libexec/alloy/alloy-core"
FRONTEND="./build/dist/usr/bin/alloy"

echo "Starting core using Unix socket at $SOCK..."
$CORE --listen "unix://$SOCK" --insecure --debug &
CORE_PID=$!

trap "kill $CORE_PID 2>/dev/null; rm -f $SOCK" EXIT
sleep 2

if [ ! -S "$SOCK" ]; then
    echo "FAILURE: Socket file $SOCK not found!"
    exit 1
fi

echo "Running version check via Unix socket..."
$FRONTEND version --socket "unix://$SOCK" --insecure

RESULT=$?

if [ $RESULT -eq 0 ]; then
    echo "SUCCESS: UDS smoke test passed (binary is reachable)!"
else
    echo "FAILURE: UDS smoke test failed!"
fi

exit $RESULT
