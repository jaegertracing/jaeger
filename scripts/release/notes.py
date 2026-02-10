#!/usr/bin/env python3

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script can read N latest commits from one of Jaeger repos
# and output them in the release notes format:
# * {title} ({author} in {pull_request})
#
# Requires personal GitHub token with default permissions:
#   https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token
#
# Usage: ./release-notes.py --help
#

import argparse
import json
import os.path
import urllib.parse
from os.path import expanduser
import sys
from urllib.request import (
    urlopen,
    Request
)
from urllib.error import HTTPError


def eprint(*args, **kwargs):
    print(*args, file=sys.stderr, **kwargs)


def print_token_error():
    """Print error message about GitHub token requirements."""
    generate_token_url = "https://github.com/settings/tokens/new?description=GitHub%20Changelog%20Generator%20token"
    eprint("\nError: Missing, invalid, or unauthorized GitHub token.")
    eprint("\nPlease ensure your GitHub token:")
    eprint("  1. Is valid and has not expired")
    eprint("  2. Has 'repo' permissions (required to access repository data)")
    eprint(f"\nTo generate a new token, visit: {generate_token_url}")
    eprint("Make sure to select the 'repo' scope when creating the token.")
    eprint("\nPlace the token in your --token-file and protect it: chmod 0600 <file>")
    sys.exit(1)


def github_api_request(url, token, additional_headers=None):
    """Make a GitHub API request with error handling.

    Args:
        url: The API URL to request
        token: GitHub personal access token
        additional_headers: Optional dict of additional headers to add

    Returns:
        Parsed JSON response
    """
    try:
        req = Request(url)
        req.add_header('Authorization', f'token {token}')
        if additional_headers:
            for header, value in additional_headers.items():
                req.add_header(header, value)
        return json.loads(urlopen(req).read())
    except HTTPError as e:
        if e.code == 401:
            print_token_error()
        raise


def num_commits_since_prev_tag(token, base_url, branch, verbose):
    tags_url = f"{base_url}/tags"
    tags = github_api_request(tags_url, token)
    prev_release_tag = tags[0]['name']
    compare_url = f"{base_url}/compare/{branch}...{prev_release_tag}"
    compare_results = github_api_request(compare_url, token)
    num_commits = compare_results['behind_by']

    if verbose:
        eprint(f"There are {num_commits} new commits since {prev_release_tag}")
    return num_commits

UNCATTEGORIZED = 'Uncategorized'
categories = [
    {'title': '#### ⛔ Breaking Changes', 'label': 'changelog:breaking-change'},
    {'title': '#### ✨ New Features', 'label': 'changelog:new-feature'},
    {'title': '#### 🐞 Bug fixes, Minor Improvements', 'label': 'changelog:bugfix-or-minor-feature'},
    {'title': '#### 🚧 Experimental Features', 'label': 'changelog:experimental'},
    {'title': '#### 👷 CI Improvements', 'label': 'changelog:ci'},
    {'title': '#### ⚙️ Refactoring', 'label': 'changelog:refactoring'},
    {'title': '#### 📖 Documentation', 'label': 'changelog:documentation'},
    {'title': None, 'label': 'changelog:test'},
    {'title': None, 'label': 'changelog:skip'},
    {'title': None, 'label': 'changelog:dependencies'},
]


def updateProgress(iteration, total_iterations):
    progress = (iteration + 1) / total_iterations
    percentage = progress * 100
    sys.stderr.write('\r[' + '='*int(progress*50) + ' '*(50-int(progress*50)) + f'] {percentage:.2f}%')
    sys.stderr.flush()
    if iteration >= total_iterations - 1:
        eprint()
    return iteration + 1

def main(token, repo, branch, num_commits, exclude_dependabot, verbose):
    accept_header = "application/vnd.github.groot-preview+json"
    base_url = f"https://api.github.com/repos/jaegertracing/{repo}"
    commits_url = f"{base_url}/commits"
    skipped_dependabot = 0

    # If num_commits isn't set, get the number of commits made since the previous release tag.
    if not num_commits:
        num_commits = num_commits_since_prev_tag(token, base_url, branch, verbose)

    if not num_commits:
        return

    # Load commits
    data = urllib.parse.urlencode({'per_page': num_commits})
    commits = github_api_request(commits_url + '?' + data, token)

    if verbose:
        eprint(req.full_url)
        eprint('Retrieved', len(commits), 'commits')

    # Load PR for each commit and print summary
    category_results = {category['title']: [] for category in categories}
    other_results = []
    commits_with_multiple_labels = []

    progress_iterator = 0
    for commit in commits:
        if verbose:
            # Update the progress bar
            progress_iterator = updateProgress(progress_iterator, num_commits)

        sha = commit['sha']
        author = commit['author']
        author_login = author['login'] if author else 'unknown'
        author_url = commit['author']['html_url'] if author else ''

        if exclude_dependabot and author == "dependabot[bot]":
            skipped_dependabot += 1
            continue

        msg_lines = commit['commit']['message'].split('\n')
        msg = msg_lines[0].capitalize()
        pulls = github_api_request(f"{commits_url}/{sha}/pulls", token, {'accept': accept_header})
        if len(pulls) > 1:
            print(f"WARNING: More than one pull request for commit {sha}")

        # Handle commits without pull requests.
        if not pulls:
            short_sha = sha[:7]
            commit_url = commit['html_url']

            result = f'* {msg} ([@{author_login}]({author_url}) in [{short_sha}]({commit_url}))'
            other_results.append(result)
            continue

        pull = pulls[0]
        pull_id = pull['number']
        pull_url = pull['html_url']
        msg = msg.replace(f'(#{pull_id})', '').strip()

        # Check if the pull request has changelog label
        pull_labels = get_pull_request_labels(token, repo, pull_id)
        changelog_labels = [label for label in pull_labels if label.startswith('changelog:')]

        # Handle multiple changelog labels
        if len(changelog_labels) > 1:
            commits_with_multiple_labels.append((sha, pull_id, changelog_labels))
            continue

        category = UNCATTEGORIZED
        if changelog_labels:
            for cat in categories:
                if changelog_labels[0].startswith(cat['label']):
                    category = cat['title']
                    break

        result = f'* {msg} ([@{author_login}]({author_url}) in [#{pull_id}]({pull_url}))'
        if category == UNCATTEGORIZED:
            other_results.append(result)
        else:
            category_results[category].append(result)

    # Print categorized pull requests
    if repo == 'jaeger':
        print()
        print('### Backend Changes')
        print()

    for category, results in category_results.items():
        if results and category:
            print(f'{category}\n')
            for result in results:
                print(result)
            print()

    # Print pull requests in the 'UNCATTEGORIZED' category
    if other_results:
        print(f'### 💩💩💩 The following commits cannot be categorized (missing "changelog:*" labels):')
        for result in other_results:
            print(result)
        print(f'### 💩💩💩 Please attach labels to these ^^^ PRs and rerun the script.')
        print(f'### 💩💩💩 Do not include this section in the changelog.')

    # Print warnings for commits with more than one changelog label
    if commits_with_multiple_labels:
        eprint("Warnings: Commits with more than one changelog label found. Please fix them:\n")
        for sha, pull_id, labels in commits_with_multiple_labels:
            pr_url = f"https://github.com/jaegertracing/{repo}/pull/{pull_id}"
            eprint(f"Commit {sha} associated with multiple changelog labels: {', '.join(labels)}")
            eprint(f"Pull Request URL: {pr_url}\n")
        print()

    if skipped_dependabot:
        if verbose:
            eprint(f"(Skipped dependabot commits: {skipped_dependabot})")


def get_pull_request_labels(token, repo, pull_number):
    labels_url = f"https://api.github.com/repos/jaegertracing/{repo}/issues/{pull_number}/labels"
    labels = github_api_request(labels_url, token)
    return [label['name'] for label in labels]


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='List changes based on git log for release notes.')

    parser.add_argument('--token-file', type=str, default="~/.github_token",
                        help='The file containing your personal github token to access the github API. ' +
                             '(default: ~/.github_token)')
    parser.add_argument('--repo', type=str, default='jaeger',
                        help='The repository name to fetch commit logs from. (default: jaeger)')
    parser.add_argument('--branch', type=str, default='main',
                        help='The branch name to fetch commit logs from. (default: main)')
    parser.add_argument('--exclude-dependabot', action='store_true',
                        help='Excludes dependabot commits. (default: false)')
    parser.add_argument('--num-commits', type=int,
                        help='Print this number of commits from git log. ' +
                             '(default: number of commits before the previous tag)')
    parser.add_argument('--verbose', action='store_true',
                        help='Whether output debug logs. (default: false)')

    args = parser.parse_args()
    token_file = expanduser(args.token_file)

    if not os.path.exists(token_file):
        eprint(f"No such token-file: {token_file}.")
        print_token_error()

    with open(token_file, 'r') as file:
        token = file.read().replace('\n', '')

    if not token:
        eprint(f"{token_file} is missing your personal github token.")
        print_token_error()

    main(token, args.repo, args.branch, args.num_commits, args.exclude_dependabot, args.verbose)
