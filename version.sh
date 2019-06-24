#!/bin/sh
dirty=""
test -z "$(git ls-files --exclude-standard --others)" || dirty="-dirty"
echo $(date "+%Y-%m-%d")-$(git rev-parse --short HEAD)${dirty}
