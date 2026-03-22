#!/bin/bash

# Alloy Project Setup Script

TEMPLATE=$1
TARGET=$2

if [ -z "$TEMPLATE" ] || [ -z "$TARGET" ]; then
    echo "Usage: $0 <template-name> <target-directory>"
    echo "Available templates:"
    ls templates/
    exit 1
fi

if [ ! -d "templates/$TEMPLATE" ]; then
    echo "Error: Template '$TEMPLATE' not found"
    exit 1
fi

echo "Creating Alloy project from template: $TEMPLATE in $TARGET"
mkdir -p "$TARGET"
cp -r templates/"$TEMPLATE"/. "$TARGET"/

echo "Done! You can now open this project with 'alloy project open $(realpath $TARGET)'"
