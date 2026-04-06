#!/bin/bash
mkdir -p .alloy/plugins
PLUGINS=(ai buffer chat documentation filesystem health iam index librarian omni-palette project repository secrets settings switcher tasks team-presence)
for p in "${PLUGINS[@]}"; do
    mkdir -p ".alloy/plugins/$p"
    echo "{}" > ".alloy/plugins/$p/config.json"
done
echo "Core .alloy structure initialized."
