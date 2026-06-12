#!/bin/sh
set -eu

version=${1:?version required}

jq --arg version "$version" '.version = $version' \
  claude-plugin/.claude-plugin/plugin.json > claude-plugin/.claude-plugin/plugin.json.tmp
mv claude-plugin/.claude-plugin/plugin.json.tmp claude-plugin/.claude-plugin/plugin.json

jq --arg version "$version" '(.plugins[] | select(.name == "sya") | .version) = $version' \
  .claude-plugin/marketplace.json > .claude-plugin/marketplace.json.tmp
mv .claude-plugin/marketplace.json.tmp .claude-plugin/marketplace.json
