# This script can read N latest commits from one of Jaeger repos
# and output them in the release notes format:
# * {title} ({author} in {pull_request})
#
# Requires personal GitHub token with default permissions:
#   https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token
#
# Usage: python release-notes.py --help
#

import argparse
import json
import urllib.parse
import os
import sys
from urllib.request import (
    urlopen,
    Request
)


def eprint(*args, **kwargs):
    print(*args, file=sys.stderr, **kwargs)


def num_commits_before_prev_tag(token, base_url):
    tags_url = f"{base_url}/tags"

    req = Request(tags_url)
    req.add_header("Authorization", f"token {token}")
    tags = json.loads(urlopen(req).read())
    prev_release_tag = tags[0]['name']
    compare_url = f"{base_url}/compare/master...{prev_release_tag}"
    req = Request(compare_url)
    req.add_header("Authorization", f"token {token}")
    compare_results = json.loads(urlopen(req).read())
    num_commits = compare_results['behind_by']

    print(f"master is behind {prev_release_tag} by {num_commits} commits")
    return num_commits


def main(token, repo, num_commits, exclude_dependabot):
    accept_header = "application/vnd.github.groot-preview+json"
    base_url = f"https://api.github.com/repos/jaegertracing/{repo}"
    commits_url = f"{base_url}/commits"
    skipped_dependabot = 0

    # If num_commits isn't set, get the number of commits made since the previous release tag.
    if not num_commits:
        num_commits = num_commits_before_prev_tag(token, base_url)

    # Load commits
    data = urllib.parse.urlencode({'per_page': num_commits})
    req = Request(commits_url + '?' + data)
    print(req.full_url)
    req.add_header('Authorization', f'token {token}')
    commits = json.loads(urlopen(req).read())
    print('Retrieved', len(commits), 'commits')

    # Load PR for each commit and print summary
    for commit in commits:
        sha = commit['sha']
        author = commit['author']['login']

        if exclude_dependabot and author == "dependabot[bot]":
            skipped_dependabot += 1
            continue

        author_url = commit['author']['html_url']
        msg_lines = commit['commit']['message'].split('\n')
        msg = msg_lines[0]
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
            print(f'* {msg} ([@{author}]({author_url}) in [{short_sha}]({commit_url}))')
            continue

        pull = pulls[0]
        pull_id = pull['number']
        pull_url = pull['html_url']
        msg = msg.replace(f'(#{pull_id})', '').strip()
        print(f'* {msg} ([@{author}]({author_url}) in [#{pull_id}]({pull_url}))')

    if skipped_dependabot:
        print(f"\n(Skipped {skipped_dependabot} dependabot commit{'' if skipped_dependabot == 1 else 's'})")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='List changes based on git log for release notes.')

    parser.add_argument('--token', type=str,
                        help='The personal github token to access the github API. ' +
                             '(default: uses OATH_TOKEN environment variable)')
    parser.add_argument('--repo', type=str, default='jaeger',
                        help='The repository name to fetch commit logs from. (default: jaeger)')
    parser.add_argument('--exclude-dependabot', action='store_true',
                        help='Excludes dependabot commits. (default: false)')
    parser.add_argument('--num-commits', type=int,
                        help='Print this number of commits from git log. ' +
                             '(default: number of commits before the previous tag)')

    args = parser.parse_args()

    token = args.token
    if not token:
        if "OAUTH_TOKEN" not in os.environ:
            eprint("Please provide a --token or set OAUTH_TOKEN environment variable to a generated token: " +
                   "https://github.com/settings/tokens/new?description=GitHub%20Changelog%20Generator%20token")
            sys.exit(1)
        token = os.environ["OAUTH_TOKEN"]

    main(token, args.repo, args.num_commits, args.exclude_dependabot)
