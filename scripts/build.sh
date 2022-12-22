#!/bin/bash

export GOOS=linux
export GOARCH=amd64
export GOARM=7
export FILENAME=arp_tracker

go build -ldflags "-s -w" -trimpath -o $FILENAME

#go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --best --lzma $FILENAME