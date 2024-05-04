#!/usr/bin/env python3

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


def eprint(*args, **kwargs):
    print(*args, file=sys.stderr, **kwargs)


def num_commits_since_prev_tag(token, base_url, verbose):
    tags_url = f"{base_url}/tags"
    trunk = "main"

    req = Request(tags_url)
    req.add_header("Authorization", f"token {token}")
    tags = json.loads(urlopen(req).read())
    prev_release_tag = tags[0]['name']
    compare_url = f"{base_url}/compare/{trunk}...{prev_release_tag}"
    req = Request(compare_url)
    req.add_header("Authorization", f"token {token}")
    compare_results = json.loads(urlopen(req).read())
    num_commits = compare_results['behind_by']

    if verbose:
        print(f"There are {num_commits} new commits since {prev_release_tag}")
    return num_commits

UNCATTEGORIZED = 'Uncategorized'
categories = [
    {'title': '#### ⛔ Breaking Changes', 'label': 'changelog:breaking-change'},
    {'title': '#### ✨ New Features', 'label': 'changelog:new-feature'},
    {'title': '#### 🐞 Bug fixes, Minor Improvements', 'label': 'changelog:bugfix-or-minor-feature'},
    {'title': '#### 🚧 Experimental Features', 'label': 'changelog:exprimental'},
    {'title': '#### 👷 CI Improvements', 'label': 'changelog:ci'},
    {'title': None, 'label': 'changelog:test'},
    {'title': None, 'label': 'changelog:skip'},
    {'title': None, 'label': 'changelog:dependencies'},
]


def updateProgress(iteration, total_iterations):
    progress = (iteration + 1) / total_iterations
    percentage = progress * 100
    sys.stdout.write('\r[' + '='*int(progress*50) + ' '*(50-int(progress*50)) + f'] {percentage:.2f}%')
    sys.stdout.flush()
    if iteration >= total_iterations - 1:
        print()
    return iteration + 1

def main(token, repo, num_commits, exclude_dependabot, verbose):
    accept_header = "application/vnd.github.groot-preview+json"
    base_url = f"https://api.github.com/repos/jaegertracing/{repo}"
    commits_url = f"{base_url}/commits"
    skipped_dependabot = 0

    # If num_commits isn't set, get the number of commits made since the previous release tag.
    if not num_commits:
        num_commits = num_commits_since_prev_tag(token, base_url, verbose)

    if not num_commits:
        return

    # Load commits
    data = urllib.parse.urlencode({'per_page': num_commits})
    req = Request(commits_url + '?' + data)
    req.add_header('Authorization', f'token {token}')
    commits = json.loads(urlopen(req).read())

    if verbose:
        print(req.full_url)
        print('Retrieved', len(commits), 'commits')

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
        author = commit['author']['login']

        if exclude_dependabot and author == "dependabot[bot]":
            skipped_dependabot += 1
            continue

        author_url = commit['author']['html_url']
        msg_lines = commit['commit']['message'].split('\n')
        msg = msg_lines[0].capitalize()
        req = Request(f"{commits_url}/{sha}/pulls")
        req.add_header('accept', accept_header)
        req.add_header('Authorization', f'token {token}')
        pulls = json.loads(urlopen(req).read())
        if len(pulls) > 1:
            print(f"WARNING: More than one pull request for commit {sha}")

        # Handle commits without pull requests.
        if not pulls:
            short_sha = sha[:7]
            commit_url = commit['html_url']

            result = f'* {msg} ([@{author}]({author_url}) in [{short_sha}]({commit_url}))'
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

        result = f'* {msg} ([@{author}]({author_url}) in [#{pull_id}]({pull_url}))'
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

    if repo == 'jaeger':
        print()
        print('### 📊 UI Changes')
        print()
        main(token, 'jaeger-ui', None, exclude_dependabot, False)

    # Print pull requests in the 'UNCATTEGORIZED' category
    if other_results:
        print(f'### 💩💩💩 The following commits cannot be categorized (missing changeglog labels):\n')
        for result in other_results:
            print(result)
        print()

    # Print warnings for commits with more than one changelog label
    if commits_with_multiple_labels:
        print("Warnings: Commits with more than one changelog label found. Please fix them:\n")
        for sha, pull_id, labels in commits_with_multiple_labels:
            pr_url = f"https://github.com/jaegertracing/{repo}/pull/{pull_id}"
            print(f"Commit {sha} associated with multiple changelog labels: {', '.join(labels)}")
            print(f"Pull Request URL: {pr_url}\n")
        print()

    if skipped_dependabot:
        print(f"(Skipped dependabot commits: {skipped_dependabot})")


def get_pull_request_labels(token, repo, pull_number):
    labels_url = f"https://api.github.com/repos/jaegertracing/{repo}/issues/{pull_number}/labels"
    req = Request(labels_url)
    req.add_header('Authorization', f'token {token}')
    labels = json.loads(urlopen(req).read())
    return [label['name'] for label in labels]


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='List changes based on git log for release notes.')

    parser.add_argument('--token-file', type=str, default="~/.github_token",
                        help='The file containing your personal github token to access the github API. ' +
                             '(default: ~/.github_token)')
    parser.add_argument('--repo', type=str, default='jaeger',
                        help='The repository name to fetch commit logs from. (default: jaeger)')
    parser.add_argument('--exclude-dependabot', action='store_true',
                        help='Excludes dependabot commits. (default: false)')
    parser.add_argument('--num-commits', type=int,
                        help='Print this number of commits from git log. ' +
                             '(default: number of commits before the previous tag)')
    parser.add_argument('--verbose', action='store_true',
                        help='Whether output debug logs. (default: false)')

    args = parser.parse_args()
    generate_token_url = "https://github.com/settings/tokens/new?description=GitHub%20Changelog%20Generator%20token"
    generate_err_msg = (f"Please generate a token from this URL: {generate_token_url} and "
                        f"place it in the token-file. Protect the file so only you can read it: chmod 0600 <file>.")

    token_file = expanduser(args.token_file)

    if not os.path.exists(token_file):
        eprint(f"No such token-file: {token_file}.\n{generate_err_msg}")
        sys.exit(1)

    with open(token_file, 'r') as file:
        token = file.read().replace('\n', '')

    if not token:
        eprint(f"{token_file} is missing your personal github token.\n{generate_err_msg}")
        sys.exit(1)

    main(token, args.repo, args.num_commits, args.exclude_dependabot, args.verbose)
