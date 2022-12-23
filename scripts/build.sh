#!/bin/bash

export GOOS=linux
export GOARCH=arm
export GOARM=5
export FILENAME=arp_tracker

#go build -ldflags "-s -w" -trimpath -o $FILENAME

go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --best --lzma $FILENAME
