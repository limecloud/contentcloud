#!/bin/sh
set -eu

freshclam --stdout
exec contentcloud-worker
