#!/bin/bash
# test_smoke.sh - Simple smoke test for Alloy plain TCP IPC

# Build latest
just build-all

PORT=9092
ADDR="127.0.0.1:$PORT"
INSTANCE="smoke-test"

# Ensure no old data
rm -rf ~/.local/share/alloy/audit.log

echo "Starting core instance '$INSTANCE' on $ADDR..."
./build/core --socket "tcp://$ADDR" --name "$INSTANCE" --insecure --debug &
CORE_PID=$!

function cleanup {
    echo "Cleaning up..."
    kill $CORE_PID 2>/dev/null
    wait $CORE_PID 2>/dev/null
}

trap cleanup EXIT

# Wait for core to be ready
MAX_WAIT=5
WAIT_COUNT=0
until ./build/frontend list | grep -q "$INSTANCE"; do
    if [ $WAIT_COUNT -ge $MAX_WAIT ]; then
        echo "ALARM: Core failed to start within $MAX_WAIT seconds"
        exit 1
    fi
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

echo "Running frontend ping..."
./build/frontend ping --socket "tcp://$ADDR" --insecure --timeout 2

RESULT=$?

if [ $RESULT -eq 0 ]; then
    echo "SUCCESS: Smoke test message flow passed!"
else
    echo "FAILURE: Smoke test message flow failed!"
fi

echo "Stopping core via CLI..."
./build/frontend stop --name "$INSTANCE"
STOP_RESULT=$?

# Wait for process to actually exit
wait $CORE_PID 2>/dev/null

if [ $RESULT -eq 0 ] && [ $STOP_RESULT -eq 0 ]; then
    echo "FULL SUCCESS: Smoke test completed fully."
    exit 0
else
    echo "FAILURE: Smoke test did not complete successfully."
    exit 1
fi
