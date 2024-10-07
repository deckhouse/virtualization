#!/usr/bin/env bash

drawio . -r -x -f png
find . -type f -name '*.png' ! -exec test -e '{}.ru.png' \; -exec sh -c 'cp "$1" "${1%.png}.ru.png"' _ {} \;
