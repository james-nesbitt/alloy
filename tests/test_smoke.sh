#!/bin/bash
# test_smoke.sh - Simple smoke test for Alloy plain TCP IPC

# Build latest
just build-all

PORT=9092
ADDR="127.0.0.1:$PORT"

echo "Starting alloy-core on $ADDR..."
./bin/alloy-core --socket "$ADDR" --debug &
CORE_PID=$!

trap "kill $CORE_PID 2>/dev/null" EXIT
sleep 2

echo "Running alloy-cli ping..."
./bin/alloy-cli ping --socket "$ADDR" --timeout 5

RESULT=$?

if [ $RESULT -eq 0 ]; then
    echo "SUCCESS: Smoke test passed!"
else
    echo "FAILURE: Smoke test failed!"
fi

exit $RESULT
