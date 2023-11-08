#!/bin/bash
set -euo pipefail

source_path="reference.md"
destination_repo="pomerium/documentation"
destination_path="content/docs/deploy/k8s"
destination_base_branch="main"
destination_head_branch="update-k8s-reference-$GITHUB_SHA"

clone_dir=$(mktemp -d)

export GITHUB_TOKEN=$API_TOKEN_GITHUB

echo "Cloning destination git repository"
git clone --depth 1 \
    "https://$API_TOKEN_GITHUB@github.com/$destination_repo.git" "$clone_dir"

echo "Copying contents to git repo"
cp -R "$source_path" "$clone_dir/$destination_path"
cd "$clone_dir"
git checkout -b "$destination_head_branch"

if [ -z "$(git status -z)" ]; then
    echo "No changes detected"
    exit
fi

echo "Adding git commit"
git config user.email "$USER_EMAIL"
git config user.name "$USER_NAME"
git add .
message="Update $destination_path from $GITHUB_REPOSITORY@$GITHUB_SHA."
git commit --message "$message"

echo "Pushing git commit"
git push -u origin HEAD:$destination_head_branch

echo "Creating a pull request"
gh pr create --title $destination_head_branch --body "$message" \
    --base $destination_base_branch --head $destination_head_branch
