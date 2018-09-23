#!/bin/sh

set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
etpdir="$workspace/src/github.com/NotoriousPyro"
if [ ! -L "$etpdir/open-metaverse-pool" ]; then
    mkdir -p "$etpdir"
    cd "$etpdir"
    ln -s ../../../../../. open-metaverse-pool
    cd "$root"
fi

# Set up the environment to use the workspace.
# Also add Godeps workspace so we build using canned dependencies.
GOPATH="$workspace"
GOBIN="$PWD/build/bin"
export GOPATH GOBIN

# Run the command inside the workspace.
cd "$etpdir/open-metaverse-pool"
PWD="$etpdir/open-metaverse-pool"

# Launch the arguments with the configured environment.
exec "$@"
