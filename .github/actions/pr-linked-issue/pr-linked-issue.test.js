// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

const handler = require('./pr-linked-issue');

const {
  processLinkedIssuePolicy,
  parseOptionalNumber,
  QUALIFYING_ISSUE_LABEL_NAME,
  MISSING_ISSUE_LABEL_NAME,
  EXEMPT_LABEL_NAME,
  STALE_LABEL_NAME,
  POLICY_COMMENT_MARKER
} = handler;

function makeIssue(number, { labels = [QUALIFYING_ISSUE_LABEL_NAME], state = 'OPEN', repository = 'owner/repo' } = {}) {
  return {
    number,
    state,
    repository: { nameWithOwner: repository, owner: { login: repository.split('/')[0] } },
    labels: { nodes: labels.map(name => ({ name })) }
  };
}

function makePR(number, { issues = [], labels = [], author = 'newcomer', state = 'OPEN' } = {}) {
  return {
    number,
    state,
    author: author ? { login: author } : null,
    labels: { nodes: labels.map(name => ({ name })) },
    closingIssuesReferences: { nodes: issues }
  };
}

function makeOctokit(pullRequests, comments = []) {
  return {
    paginate: jest.fn().mockResolvedValue(comments),
    graphql: jest.fn((query, { number }) => Promise.resolve({
      repository: {
        pullRequests: { nodes: pullRequests.filter(pr => pr.state === 'OPEN'), pageInfo: { hasNextPage: false } },
        pullRequest: pullRequests.find(pr => pr.number === number)
      }
    })),
    rest: {
      repos: {
        getCollaboratorPermissionLevel: jest.fn(({ username }) =>
          Promise.resolve({ data: { permission: username === 'maintainer' ? 'write' : 'read' } })
        )
      },
      issues: {
        listComments: jest.fn(),
        createComment: jest.fn().mockResolvedValue({}),
        updateComment: jest.fn().mockResolvedValue({}),
        addLabels: jest.fn().mockResolvedValue({}),
        removeLabel: jest.fn().mockResolvedValue({})
      }
    }
  };
}

function policyComment(id, text, authorFields = { user: { login: 'github-actions[bot]' } }) {
  return { id, body: `${POLICY_COMMENT_MARKER}\n${text}`, ...authorFields };
}

function applyPolicy(octokit, options) {
  return processLinkedIssuePolicy(octokit, 'owner', 'repo', options, { log: () => {} });
}

describe('which pull requests violate the policy', () => {
  test.each([
    { name: 'no linked issue', pr: {}, flagged: true },
    { name: 'linked issue without the label', pr: { issues: [makeIssue(7, { labels: ['bug'] })] }, flagged: true },
    { name: 'linked issue is closed', pr: { issues: [makeIssue(7, { state: 'CLOSED' })] }, flagged: true },
    { name: 'labelled issue of another organization', pr: { issues: [makeIssue(7, { repository: 'elsewhere/repo' })] }, flagged: true },
    { name: 'bot author that is not a dependency bot', pr: { author: 'coding-agent' }, flagged: true },
    { name: 'deleted author', pr: { author: null }, flagged: true },
    { name: 'open labelled issue', pr: { issues: [makeIssue(7)] }, flagged: false },
    { name: 'one of several linked issues is labelled', pr: { issues: [makeIssue(7, { labels: [] }), makeIssue(8)] }, flagged: false },
    { name: 'labelled issue in another repository of the organization', pr: { issues: [makeIssue(7, { repository: 'owner/other' })] }, flagged: false },
    { name: 'Dependabot author', pr: { author: 'dependabot' }, flagged: false },
    { name: 'Renovate author', pr: { author: 'renovate-bot' }, flagged: false },
    { name: 'exempt label', pr: { labels: [EXEMPT_LABEL_NAME] }, flagged: false },
    { name: 'author with write access', pr: { author: 'maintainer' }, flagged: false }
  ])('$name', async ({ pr, flagged }) => {
    const octokit = makeOctokit([makePR(1, pr)]);

    const result = await applyPolicy(octokit);

    expect(result).toEqual({ flaggedPullRequests: flagged ? [1] : [], unflaggedPullRequests: [] });
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledTimes(flagged ? 1 : 0);
  });
});

describe('flagging a pull request', () => {
  test('posts the comment before adding both labels', async () => {
    const octokit = makeOctokit([makePR(1)]);

    await applyPolicy(octokit);

    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith({
      owner: 'owner', repo: 'repo', issue_number: 1, labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]
    });
    expect(octokit.rest.issues.createComment.mock.invocationCallOrder[0])
      .toBeLessThan(octokit.rest.issues.addLabels.mock.invocationCallOrder[0]);
  });

  test.each([
    { name: 'no issue is linked', issues: [], problems: ['no issue is linked'] },
    { name: 'the issue lacks the label', issues: [makeIssue(7, { labels: ['bug'] })], problems: ['#7 is not labelled `help wanted`'] },
    { name: 'the issue is closed', issues: [makeIssue(7, { state: 'CLOSED' })], problems: ['#7 is closed'] },
    {
      name: 'the issue belongs to another organization',
      issues: [makeIssue(7, { repository: 'elsewhere/repo' })],
      problems: ['elsewhere/repo#7 belongs to another organization']
    },
    {
      name: 'several issues fail for different reasons',
      issues: [makeIssue(7, { state: 'CLOSED' }), makeIssue(8, { labels: [], repository: 'owner/other' })],
      problems: ['#7 is closed', 'owner/other#8 is not labelled `help wanted`']
    }
  ])('the comment and the log say that $name', async ({ issues, problems }) => {
    const octokit = makeOctokit([makePR(1, { issues })]);
    const logger = { log: jest.fn() };

    await processLinkedIssuePolicy(octokit, 'owner', 'repo', {}, logger);

    const { body } = octokit.rest.issues.createComment.mock.calls[0][0];
    expect(body).toContain(problems.map(problem => `- ${problem}`).join('\n'));
    expect(body).toContain('CONTRIBUTING_GUIDELINES.md#link-each-pull-request-to-an-issue');
    expect(logger.log).toHaveBeenCalledWith(`PR #1 by @newcomer: ${problems.join('; ')}`);
  });

  test('posts a new comment when a pull request is flagged again', async () => {
    const octokit = makeOctokit([makePR(1)], [policyComment(56, 'now meets the policy')]);

    await applyPolicy(octokit);

    expect(octokit.rest.issues.createComment).toHaveBeenCalledTimes(1);
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();
  });

  test('rewrites the latest policy comment when the linked issues of a flagged pull request change', async () => {
    const pr = makePR(1, {
      issues: [makeIssue(7, { labels: [] })],
      labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]
    });
    const octokit = makeOctokit([pr], [policyComment(54, 'older'), policyComment(56, 'No issue is linked')]);

    await applyPolicy(octokit);

    const [{ comment_id: commentId, body }] = octokit.rest.issues.updateComment.mock.calls[0];
    expect(commentId).toBe(56);
    expect(body).toContain('- #7 is not labelled `help wanted`');
    expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalled();
  });

  test('restores the stale label on a flagged pull request that lost it', async () => {
    const octokit = makeOctokit([makePR(1, { labels: [MISSING_ISSUE_LABEL_NAME] })]);

    await applyPolicy(octokit);

    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({ labels: [STALE_LABEL_NAME] }));
  });

  test.each([
    { name: 'the workflow account', authorFields: { user: { login: 'github-actions[bot]' } }, rewritten: true },
    { name: 'the repository owner', authorFields: { author_association: 'OWNER' }, rewritten: true },
    { name: 'an organization member', authorFields: { author_association: 'MEMBER' }, rewritten: true },
    { name: 'a collaborator', authorFields: { author_association: 'COLLABORATOR' }, rewritten: true },
    { name: 'another bot', authorFields: { user: { login: 'coding-agent[bot]' }, author_association: 'NONE' }, rewritten: false },
    { name: 'a contributor', authorFields: { author_association: 'CONTRIBUTOR' }, rewritten: false }
  ])('a marked comment from $name is rewritten: $rewritten', async ({ authorFields, rewritten }) => {
    const pr = makePR(1, { labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME] });
    const octokit = makeOctokit([pr], [policyComment(56, 'outdated', authorFields)]);

    await applyPolicy(octokit);

    expect(octokit.rest.issues.updateComment).toHaveBeenCalledTimes(rewritten ? 1 : 0);
  });

  test('looks up the permission of an author once', async () => {
    const octokit = makeOctokit([makePR(1), makePR(2)]);

    await applyPolicy(octokit);

    expect(octokit.rest.repos.getCollaboratorPermissionLevel).toHaveBeenCalledTimes(1);
  });
});

describe('a pull request that meets the policy', () => {
  test('loses both labels when it was flagged', async () => {
    const octokit = makeOctokit([
      makePR(1, { issues: [makeIssue(7)], labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME] })
    ]);

    const result = await applyPolicy(octokit);

    expect(result).toEqual({ flaggedPullRequests: [], unflaggedPullRequests: [1] });
    expect(octokit.rest.issues.removeLabel.mock.calls.map(([args]) => args.name))
      .toEqual([STALE_LABEL_NAME, MISSING_ISSUE_LABEL_NAME]);
  });

  test('has its policy comment rewritten to say so', async () => {
    const pr = makePR(1, { issues: [makeIssue(7)], labels: [MISSING_ISSUE_LABEL_NAME] });
    const octokit = makeOctokit([pr], [policyComment(56, 'No issue is linked')]);

    await applyPolicy(octokit);

    const [{ comment_id: commentId, body }] = octokit.rest.issues.updateComment.mock.calls[0];
    expect(commentId).toBe(56);
    expect(body).toContain(POLICY_COMMENT_MARKER);
    expect(body).toContain('now meets the linked issue policy');
    expect(octokit.rest.issues.updateComment.mock.invocationCallOrder[0])
      .toBeLessThan(octokit.rest.issues.removeLabel.mock.invocationCallOrder[0]);
  });

  test('keeps a policy comment that already says so', async () => {
    const pr = makePR(1, { issues: [makeIssue(7)], labels: [MISSING_ISSUE_LABEL_NAME] });
    const octokit = makeOctokit([pr], [policyComment(56, 'This pull request now meets the linked issue policy.')]);

    await applyPolicy(octokit);

    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();
  });

  test('keeps the stale label when it was never flagged', async () => {
    const octokit = makeOctokit([makePR(1, { issues: [makeIssue(7)], labels: [STALE_LABEL_NAME] })]);

    await applyPolicy(octokit);

    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });
});

describe('which pull requests a run checks', () => {
  const pullRequests = [
    makePR(1, { issues: [makeIssue(7, { labels: [] })] }),
    makePR(2, { issues: [makeIssue(7, { labels: [], repository: 'owner/other' })] }),
    makePR(3),
    makePR(4, { state: 'CLOSED' })
  ];

  test.each([
    { name: 'every open pull request by default', options: undefined, flagged: [1, 2, 3] },
    { name: 'the named pull request', options: { pullRequestNumber: 3 }, flagged: [3] },
    { name: 'nothing when the named pull request is closed', options: { pullRequestNumber: 4 }, flagged: [] }
  ])('$name', async ({ options, flagged }) => {
    const result = await applyPolicy(makeOctokit(pullRequests), options);

    expect(result.flaggedPullRequests).toEqual(flagged);
  });

  test('reads every page of open pull requests', async () => {
    const octokit = makeOctokit([]);
    octokit.graphql
      .mockResolvedValueOnce({
        repository: { pullRequests: { nodes: [makePR(1)], pageInfo: { hasNextPage: true, endCursor: 'cursor' } } }
      })
      .mockResolvedValueOnce({
        repository: { pullRequests: { nodes: [makePR(2)], pageInfo: { hasNextPage: false } } }
      });

    const result = await applyPolicy(octokit);

    expect(result.flaggedPullRequests).toEqual([1, 2]);
    expect(octokit.graphql).toHaveBeenLastCalledWith(expect.any(String), { owner: 'owner', repo: 'repo', cursor: 'cursor' });
  });
});

test('a dry run reports the plan and writes nothing', async () => {
  const octokit = makeOctokit([
    makePR(1),
    makePR(2, { issues: [makeIssue(7)], labels: [MISSING_ISSUE_LABEL_NAME] })
  ]);

  const result = await applyPolicy(octokit, { dryRun: true });

  expect(result).toEqual({ flaggedPullRequests: [1], unflaggedPullRequests: [2] });
  for (const method of ['createComment', 'updateComment', 'addLabels', 'removeLabel']) {
    expect(octokit.rest.issues[method]).not.toHaveBeenCalled();
  }
});

describe('parseOptionalNumber', () => {
  test.each([
    { value: undefined, expected: undefined },
    { value: '', expected: undefined },
    { value: '42', expected: 42 }
  ])('reads $value as $expected', ({ value, expected }) => {
    expect(parseOptionalNumber(value, 'pull-request-number')).toBe(expected);
  });

  test.each(['#42', 'abc', '0', '-1', '4.2'])('rejects %s', value => {
    expect(() => parseOptionalNumber(value, 'pull-request-number'))
      .toThrow(`pull-request-number must be a positive integer, got "${value}"`);
  });
});

describe('GitHub Actions handler', () => {
  let logSpy;
  beforeEach(() => {
    logSpy = jest.spyOn(console, 'log').mockImplementation(() => {});
  });
  afterEach(() => {
    logSpy.mockRestore();
  });

  test('checks the pull request named by a string input and reports the result', async () => {
    const github = makeOctokit([makePR(1), makePR(2)]);
    const core = { info: jest.fn(), setFailed: jest.fn() };

    await handler({ github, core, owner: 'owner', repo: 'repo', pullRequestNumber: '2' });

    expect(github.rest.issues.addLabels).toHaveBeenCalledTimes(1);
    expect(core.info).toHaveBeenCalledWith('PRs that violate the policy: 2');
    expect(core.info).toHaveBeenCalledWith('PRs that now meet the policy: none');
  });

  test('marks a dry run in the report', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };

    await handler({ github: makeOctokit([]), core, owner: 'owner', repo: 'repo', dryRun: true });

    expect(core.info).toHaveBeenCalledWith('Dry-run plan: PRs that violate the policy: none');
  });

  test('fails the step on a malformed pull request number and checks nothing', async () => {
    const github = makeOctokit([makePR(1)]);
    const core = { info: jest.fn(), setFailed: jest.fn() };

    await expect(handler({ github, core, owner: 'owner', repo: 'repo', pullRequestNumber: '#1' }))
      .rejects.toThrow('pull-request-number must be a positive integer');
    expect(github.graphql).not.toHaveBeenCalled();
    expect(core.setFailed).toHaveBeenCalled();
  });
});
