#!/bin/bash

if [[ ${TESTLEAK_MODE} = "todo" ]]; then
    # TODO
else
    exec valgrind --leak-check=full --show-leak-kinds=all --track-origins=yes ./internal/testleak/tmp/app-docker ${TESTLEAK_ARGS}
fi