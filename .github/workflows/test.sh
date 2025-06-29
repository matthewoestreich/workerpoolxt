#
#
#
#
# THIS IS OLD BUT I AM KEEPING JUST IN CASE
#
#
#
#
#
#
#











#!/usr/bin/env bash
# ====================================================================== #
# Thanks to https://github.com/gammazero/workerpool for the inspiration! #
# ====================================================================== #
set -e

profile=profile.out; coverage=coverage.out; special_count=2000;

echo "" >"$coverage"

for d in $(go list ./... | grep -v vendor); do
    # Basic, defaut test that generates codecov
    go test -v -race -coverprofile="$profile" -covermode=atomic "$d"
    printf "\n%s\n" "[STARTING] : '^Test.*_Special$' : this may take a while"
    # Includes any test ending in `_Special`
    go test -race -run ^Test.*_Special$ -count="$special_count" "$d"
    # Cleanup
    if [ -f "$profile" ]; then
        cat "$profile" >>"$coverage"
        rm "$profile"
    fi
done
