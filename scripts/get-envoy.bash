#!/bin/bash
set -euo pipefail

PATH="$PATH:$(go env GOPATH)/bin"
export PATH

_project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/.."
_envoy_version=1.23.0
_dir="$_project_root/pomerium/envoy/bin"
_target="${TARGET:-"$(go env GOOS)-$(go env GOARCH)"}"

_url="https://github.com/pomerium/envoy-binaries/releases/download/v${_envoy_version}/envoy-${_target}"

mkdir -p "$_dir"

curl \
    --compressed \
    --silent \
    --fail \
    --location \
    --time-cond "$_dir/envoy-$_target" \
    --output "$_dir/envoy-$_target" \
    "$_url"

curl \
    --compressed \
    --silent \
    --fail \
    --location \
    --time-cond "$_dir/envoy-$_target.sha256" \
    --output "$_dir/envoy-$_target.sha256" \
    "$_url.sha256"

echo "$_envoy_version" >"$_dir/envoy-$_target.version"
