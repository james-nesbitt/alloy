#!/bin/bash
set -e

gen_manifest() {
    local id=$1
    local pattern=${2:-multi}
    cat <<EOF > "plugins/wasm/$id/$id.json"
{
  "id": "$id",
  "load_time": "lazy",
  "instance_pattern": "$pattern"
}
EOF
    echo "Generated manifest for $id ($pattern)"
}

# Standard App Logic (Multi-instance)
gen_manifest ai
gen_manifest buffer
gen_manifest chat
gen_manifest documentation
gen_manifest index
gen_manifest librarian
gen_manifest project
gen_manifest repository
gen_manifest secrets
gen_manifest settings
gen_manifest switcher
gen_manifest tasks
gen_manifest team-presence
gen_manifest omni-palette

# Providers / Tools
gen_manifest health mono
# filesystem.json already exists

echo "All manifests generated."
