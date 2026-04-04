#!/bin/bash
set -e

echo "--- RUNNING SANITY BUILD VIA JUST ---"
just build-all

echo "--- SANITY BUILD COMPLETE ---"
