#!/bin/bash
# test_uds.sh - Test Alloy using Unix Domain Sockets

just build-all

SOCK="./alloy.sock"
rm -f "$SOCK"

echo "Starting alloy-core using Unix socket at $SOCK..."
./bin/alloy-core --socket "$SOCK" --debug &
CORE_PID=$!

trap "kill $CORE_PID 2>/dev/null; rm -f $SOCK" EXIT
sleep 2

if [ ! -S "$SOCK" ]; then
    echo "FAILURE: Socket file $SOCK not found!"
    exit 1
fi

echo "Running alloy-cli ping via Unix socket..."
./bin/alloy-cli ping --socket "$SOCK" --timeout 5

RESULT=$?

if [ $RESULT -eq 0 ]; then
    echo "SUCCESS: UDS smoke test passed!"
else
    echo "FAILURE: UDS smoke test failed!"
fi

exit $RESULT
