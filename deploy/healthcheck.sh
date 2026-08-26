#!/bin/sh
set -eu
wget -q --spider "${1:?health URL is required}"
