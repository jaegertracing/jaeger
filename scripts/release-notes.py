
# This script can read N latest commits from one of Jaeger repos
# and output them in the release notes format:
#   {title} ({author} in {pull_request})
#
# Requires personal GitHub token with default permissions:
#   https://docs.github.com/en/github/authenticating-to-github/keeping-your-account-and-data-secure/creating-a-personal-access-token
#
# Usage: python release-notes.py <github_token> <jaeger-repo> <num-commits>
#

import json
from urllib.request import (
    urlopen,
    Request
)
import urllib.parse
import sys

def eprint(*args, **kwargs):
    print(*args, file=sys.stderr, **kwargs)

if len(sys.argv) < 4:
    eprint("Usage: python release-notes.py <github_token> <jaeger-repo> <num-commits>")
    sys.exit(1)

token=sys.argv[1]
repo=sys.argv[2]
num_commits=sys.argv[3]

accept_header="application/vnd.github.groot-preview+json"
commits_url='https://api.github.com/repos/jaegertracing/{repo}/commits'.format(repo=repo)
pulls_url="{commits_url}/{commit_sha}/pulls"

# load commits
data = urllib.parse.urlencode({'per_page': num_commits})
req=Request(commits_url + '?' + data)
print(req.full_url)
req.add_header('Authorization', 'token {token}'.format(token=token))
commits=json.loads(urlopen(req).read())
print('Retrieved', len(commits), 'commits')

# load PR for each commit and print summary
for commit in commits:
    sha = commit['sha']
    msg_lines = commit['commit']['message'].split('\n')
    msg = msg_lines[0]
    req = Request(pulls_url.format(commits_url=commits_url,commit_sha=sha))
    req.add_header('accept', accept_header)
    req.add_header('Authorization', 'token {token}'.format(token=token))
    pulls = json.loads(urlopen(req).read())
    if len(pulls) > 1:
        print("More than one pull request for commit {commit}".format(commit=sha))
    # TODO handle commits without pull requests
    pull = pulls[0]
    pull_no = pull['number']
    pull_url = pull['html_url']
    author = pull['user']['login']
    author_url = pull['user']['html_url']
    msg = msg.replace('(#{pull})'.format(pull=pull_no), '').strip()
    print('* {msg} ([@{author}]({author_url}) in [#{pull}]({pull_url}))'.format(
        msg=msg,
        author=author,
        author_url=author_url,
        pull=pull_no,
        pull_url=pull_url,
    ))
