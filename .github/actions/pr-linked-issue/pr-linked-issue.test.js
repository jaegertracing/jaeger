// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

const handler = require('./pr-linked-issue');

const {
  processLinkedIssuePolicy,
  parseOptionalNumber,
  missingIssueComment,
  policyMetComment,
  QUALIFYING_ISSUE_LABEL_NAME,
  MISSING_ISSUE_LABEL_NAME,
  EXEMPT_LABEL_NAME,
  STALE_LABEL_NAME,
  POLICY_COMMENT_MARKER
} = handler;

const silentLogger = { log: () => {} };

function makeIssue(number, { labels = [QUALIFYING_ISSUE_LABEL_NAME], state = 'OPEN', repository = 'owner/repo' } = {}) {
  return {
    number,
    state,
    repository: { nameWithOwner: repository, owner: { login: repository.split('/')[0] } },
    labels: { nodes: labels.map(name => ({ name })) }
  };
}

function makePR(number, {
  issues = [], labels = [], author = 'newcomer', state = 'OPEN'
} = {}) {
  return {
    number,
    state,
    author: author ? { login: author } : null,
    labels: { nodes: labels.map(name => ({ name })) },
    closingIssuesReferences: { nodes: issues }
  };
}

function policyComment(body, authorFields = { user: { login: 'github-actions[bot]' } }) {
  return { id: 55, body, ...authorFields };
}

function makeOctokit({ pullRequests, comments = {}, permissions = {}, labelsAbsentFromRepository = [] }) {
  return {
    graphql: jest.fn((query, { number }) => Promise.resolve({
      repository: {
        pullRequests: { nodes: pullRequests, pageInfo: { hasNextPage: false } },
        pullRequest: pullRequests.find(pr => pr.number === number) || null
      }
    })),
    rest: {
      repos: {
        getCollaboratorPermissionLevel: jest.fn(({ username }) =>
          Promise.resolve({ data: { permission: permissions[username] || 'read' } })
        )
      },
      issues: {
        listComments: jest.fn(({ issue_number }) => Promise.resolve({ data: comments[issue_number] || [] })),
        createComment: jest.fn().mockResolvedValue({}),
        updateComment: jest.fn().mockResolvedValue({}),
        getLabel: jest.fn(({ name }) =>
          labelsAbsentFromRepository.includes(name) ? Promise.reject({ status: 404 }) : Promise.resolve({})
        ),
        createLabel: jest.fn().mockResolvedValue({}),
        addLabels: jest.fn().mockResolvedValue({}),
        removeLabel: jest.fn().mockResolvedValue({})
      }
    }
  };
}

function applyPolicy(octokit, options) {
  return processLinkedIssuePolicy(octokit, 'owner', 'repo', options, silentLogger);
}

describe('which pull requests violate the policy', () => {
  const cases = [
    { name: 'no linked issue', pr: {}, flagged: true },
    { name: 'linked issue without the label', pr: { issues: [makeIssue(7, { labels: ['bug'] })] }, flagged: true },
    { name: 'linked issue is closed', pr: { issues: [makeIssue(7, { state: 'CLOSED' })] }, flagged: true },
    {
      name: 'labelled issue belongs to another organization',
      pr: { issues: [makeIssue(7, { repository: 'elsewhere/repo' })] },
      flagged: true
    },
    { name: 'open labelled issue', pr: { issues: [makeIssue(7)] }, flagged: false },
    {
      name: 'labelled issue whose owner is spelled in another case',
      pr: { issues: [makeIssue(7, { repository: 'Owner/repo' })] },
      flagged: false
    },
    {
      name: 'one of several linked issues is labelled',
      pr: { issues: [makeIssue(7, { labels: [] }), makeIssue(8)] },
      flagged: false
    },
    {
      name: 'labelled issue in another repository of the organization',
      pr: { issues: [makeIssue(7, { repository: 'owner/other' })] },
      flagged: false
    },
    { name: 'dependency bot author', pr: { author: 'dependabot' }, flagged: false },
    { name: 'bot author that is not a dependency bot', pr: { author: 'coding-agent' }, flagged: true },
    { name: 'exempt label', pr: { labels: [EXEMPT_LABEL_NAME] }, flagged: false },
    { name: 'author with write access', pr: { author: 'maintainer' }, flagged: false },
    { name: 'author with admin access', pr: { author: 'admin' }, flagged: false },
    { name: 'deleted author', pr: { author: null }, flagged: true }
  ];

  test.each(cases)('$name', async ({ pr, flagged }) => {
    const octokit = makeOctokit({
      pullRequests: [makePR(1, pr)],
      permissions: { maintainer: 'write', admin: 'admin' }
    });

    const result = await applyPolicy(octokit);

    expect(result).toEqual({ flaggedPullRequests: flagged ? [1] : [], unflaggedPullRequests: [] });
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledTimes(flagged ? 1 : 0);
  });
});

describe('flagging a pull request', () => {
  test('posts the comment before adding both labels', async () => {
    const pr = makePR(1);
    const octokit = makeOctokit({ pullRequests: [pr] });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.createComment).toHaveBeenCalledWith({
      owner: 'owner', repo: 'repo', issue_number: 1, body: missingIssueComment(pr, 'owner/repo')
    });
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith({
      owner: 'owner', repo: 'repo', issue_number: 1, labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]
    });
    expect(octokit.rest.issues.createComment.mock.invocationCallOrder[0])
      .toBeLessThan(octokit.rest.issues.addLabels.mock.invocationCallOrder[0]);
  });

  test('creates the policy labels that the repository lacks', async () => {
    const octokit = makeOctokit({ pullRequests: [makePR(1)], labelsAbsentFromRepository: [EXEMPT_LABEL_NAME] });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.createLabel).toHaveBeenCalledTimes(1);
    expect(octokit.rest.issues.createLabel).toHaveBeenCalledWith(expect.objectContaining({ name: EXEMPT_LABEL_NAME }));
  });

  test('fails when a label lookup fails for a reason other than a missing label', async () => {
    const octokit = makeOctokit({ pullRequests: [makePR(1)] });
    octokit.rest.issues.getLabel.mockRejectedValue({ status: 500 });

    await expect(applyPolicy(octokit)).rejects.toEqual({ status: 500 });
  });

  test('rewrites the existing comment when the linked issues changed', async () => {
    const pr = makePR(1, {
      issues: [makeIssue(7, { labels: [] })],
      labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]
    });
    const octokit = makeOctokit({
      pullRequests: [pr],
      comments: { 1: [policyComment(missingIssueComment(makePR(1), 'owner/repo'))] }
    });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith({
      owner: 'owner', repo: 'repo', comment_id: 55, body: missingIssueComment(pr, 'owner/repo')
    });
    expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalled();
  });

  test('posts a new comment when a pull request is flagged again', async () => {
    const octokit = makeOctokit({
      pullRequests: [makePR(1)],
      comments: { 1: [policyComment(policyMetComment())] }
    });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.createComment).toHaveBeenCalledTimes(1);
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();
  });

  test('rewrites the latest of several policy comments', async () => {
    const pr = makePR(1, { labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME] });
    const octokit = makeOctokit({
      pullRequests: [pr],
      comments: {
        1: [
          { ...policyComment(policyMetComment()), id: 54 },
          { ...policyComment(`${POLICY_COMMENT_MARKER}\nold`), id: 56 }
        ]
      }
    });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(expect.objectContaining({ comment_id: 56 }));
  });

  test('restores only the stale label on a pull request that is already flagged', async () => {
    const pr = makePR(1, { labels: [MISSING_ISSUE_LABEL_NAME] });
    const octokit = makeOctokit({
      pullRequests: [pr],
      comments: { 1: [policyComment(missingIssueComment(pr, 'owner/repo'))] }
    });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();
    expect(octokit.rest.issues.getLabel).not.toHaveBeenCalled();
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({ labels: [STALE_LABEL_NAME] }));
  });

  test('finds the policy comment past the first page of comments', async () => {
    const pr = makePR(1, { labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME] });
    const octokit = makeOctokit({ pullRequests: [pr] });
    octokit.rest.issues.listComments
      .mockResolvedValueOnce({ data: Array.from({ length: 100 }, () => ({ body: 'unrelated' })) })
      .mockResolvedValueOnce({ data: [policyComment(missingIssueComment(pr, 'owner/repo'))] });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
  });

  test('looks up the permission of an author once', async () => {
    const octokit = makeOctokit({ pullRequests: [makePR(1), makePR(2)] });

    const result = await applyPolicy(octokit);

    expect(result.flaggedPullRequests).toEqual([1, 2]);
    expect(octokit.rest.repos.getCollaboratorPermissionLevel).toHaveBeenCalledTimes(1);
  });
});

describe('the policy comments', () => {
  test('a pull request without a linked issue', () => {
    expect(missingIssueComment(makePR(1), 'owner/repo')).toMatchSnapshot();
  });

  test('a pull request whose linked issues do not qualify', () => {
    const pr = makePR(1, {
      issues: [makeIssue(7, { labels: [] }), makeIssue(8, { labels: [], repository: 'owner/other' })]
    });

    expect(missingIssueComment(pr, 'owner/repo')).toMatchSnapshot();
  });

  test('a pull request that meets the policy again', () => {
    expect(policyMetComment()).toMatchSnapshot();
  });
});

describe('which comment the action maintains', () => {
  const flaggedPR = makePR(1, { labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME] });
  const outdatedBody = `${POLICY_COMMENT_MARKER}\nold`;

  test.each([
    { name: 'a comment from the workflow account', authorFields: { user: { login: 'github-actions[bot]' } }, maintained: true },
    { name: 'a comment from the repository owner', authorFields: { user: { login: 'someone' }, author_association: 'OWNER' }, maintained: true },
    { name: 'a comment from an organization member', authorFields: { user: { login: 'someone' }, author_association: 'MEMBER' }, maintained: true },
    { name: 'a comment from a collaborator', authorFields: { user: { login: 'someone' }, author_association: 'COLLABORATOR' }, maintained: true },
    { name: 'a comment from another bot', authorFields: { user: { login: 'coding-agent[bot]' }, author_association: 'NONE' }, maintained: false },
    { name: 'a comment from a contributor', authorFields: { user: { login: 'someone' }, author_association: 'CONTRIBUTOR' }, maintained: false }
  ])('$name that carries the marker', async ({ authorFields, maintained }) => {
    const octokit = makeOctokit({
      pullRequests: [flaggedPR],
      comments: { 1: [policyComment(outdatedBody, authorFields)] }
    });

    await applyPolicy(octokit);

    expect(octokit.rest.issues.updateComment).toHaveBeenCalledTimes(maintained ? 1 : 0);
    expect(octokit.rest.issues.createComment).toHaveBeenCalledTimes(maintained ? 0 : 1);
  });
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

describe('unflagging a pull request', () => {
  test('removes both labels and resolves the comment', async () => {
    const octokit = makeOctokit({
      pullRequests: [makePR(1, { issues: [makeIssue(7)], labels: [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME] })],
      comments: { 1: [policyComment(`${POLICY_COMMENT_MARKER}\nold`)] }
    });

    const result = await applyPolicy(octokit);

    expect(result).toEqual({ flaggedPullRequests: [], unflaggedPullRequests: [1] });
    expect(octokit.rest.issues.removeLabel.mock.calls.map(([args]) => args.name))
      .toEqual([MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]);
    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith({
      owner: 'owner', repo: 'repo', comment_id: 55, body: policyMetComment()
    });
  });

  test('leaves the stale label alone on a pull request that was never flagged', async () => {
    const octokit = makeOctokit({
      pullRequests: [makePR(1, { issues: [makeIssue(7)], labels: [STALE_LABEL_NAME] })]
    });

    const result = await applyPolicy(octokit);

    expect(result).toEqual({ flaggedPullRequests: [], unflaggedPullRequests: [] });
    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });

  test('tolerates a label that is already gone and a comment that is already resolved', async () => {
    const octokit = makeOctokit({
      pullRequests: [makePR(1, { labels: [MISSING_ISSUE_LABEL_NAME, EXEMPT_LABEL_NAME] })],
      comments: { 1: [policyComment(policyMetComment())] }
    });
    octokit.rest.issues.removeLabel.mockRejectedValue({ status: 404 });

    await expect(applyPolicy(octokit)).resolves.toEqual({ flaggedPullRequests: [], unflaggedPullRequests: [1] });
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();
  });

  test('fails when a label cannot be removed', async () => {
    const octokit = makeOctokit({
      pullRequests: [makePR(1, { issues: [makeIssue(7)], labels: [MISSING_ISSUE_LABEL_NAME] })]
    });
    octokit.rest.issues.removeLabel.mockRejectedValue({ status: 403 });

    await expect(applyPolicy(octokit)).rejects.toEqual({ status: 403 });
  });
});

describe('which pull requests a run checks', () => {
  const pullRequests = [
    makePR(1, { issues: [makeIssue(7, { labels: [] })] }),
    makePR(2, { issues: [makeIssue(7, { labels: [], repository: 'owner/other' })] }),
    makePR(3)
  ];

  test.each([
    { name: 'every open pull request by default', options: undefined, flagged: [1, 2, 3] },
    { name: 'the named pull request', options: { pullRequestNumber: 3 }, flagged: [3] },
    { name: 'the pull requests linked to the named issue of this repository', options: { issueNumber: 7 }, flagged: [1] },
    { name: 'the named pull request when both numbers are given', options: { pullRequestNumber: 3, issueNumber: 7 }, flagged: [3] }
  ])('$name', async ({ options, flagged }) => {
    const result = await applyPolicy(makeOctokit({ pullRequests }), options);

    expect(result.flaggedPullRequests).toEqual(flagged);
  });

  test.each([
    { name: 'is closed', pullRequests: [makePR(3, { state: 'CLOSED' })] },
    { name: 'does not exist', pullRequests: [] }
  ])('nothing when the named pull request $name', async ({ pullRequests }) => {
    const octokit = makeOctokit({ pullRequests });

    const result = await applyPolicy(octokit, { pullRequestNumber: 3 });

    expect(result).toEqual({ flaggedPullRequests: [], unflaggedPullRequests: [] });
    expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
  });

  test('reads only the named pull request', async () => {
    const octokit = makeOctokit({ pullRequests });

    await applyPolicy(octokit, { pullRequestNumber: 3 });

    expect(octokit.graphql).toHaveBeenCalledTimes(1);
    expect(octokit.graphql).toHaveBeenCalledWith(expect.any(String), { owner: 'owner', repo: 'repo', number: 3 });
  });

  test('reads every page of open pull requests', async () => {
    const octokit = makeOctokit({ pullRequests: [] });
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
  const octokit = makeOctokit({
    pullRequests: [makePR(1), makePR(2, { issues: [makeIssue(7)], labels: [MISSING_ISSUE_LABEL_NAME] })]
  });
  const logger = { log: jest.fn() };

  const result = await processLinkedIssuePolicy(octokit, 'owner', 'repo', { dryRun: true }, logger);

  expect(result).toEqual({ flaggedPullRequests: [1], unflaggedPullRequests: [2] });
  expect(logger.log).toHaveBeenCalledTimes(2);
  for (const method of ['createComment', 'updateComment', 'createLabel', 'addLabels', 'removeLabel']) {
    expect(octokit.rest.issues[method]).not.toHaveBeenCalled();
  }
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
    const github = makeOctokit({ pullRequests: [makePR(1), makePR(2)] });
    const core = { info: jest.fn(), setFailed: jest.fn() };

    await handler({ github, core, owner: 'owner', repo: 'repo', pullRequestNumber: '2', issueNumber: '' });

    expect(github.rest.issues.addLabels).toHaveBeenCalledTimes(1);
    expect(github.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({ issue_number: 2 }));
    expect(core.info).toHaveBeenCalledWith('PRs without a qualifying issue: 2');
    expect(core.info).toHaveBeenCalledWith('PRs that now meet the policy: none');
  });

  test('marks a dry run in the report', async () => {
    const github = makeOctokit({ pullRequests: [] });
    const core = { info: jest.fn(), setFailed: jest.fn() };

    await handler({ github, core, owner: 'owner', repo: 'repo', dryRun: true });

    expect(core.info).toHaveBeenCalledWith('Dry-run plan: PRs without a qualifying issue: none');
  });

  test('fails the step on a malformed pull request number and checks nothing', async () => {
    const github = makeOctokit({ pullRequests: [makePR(1)] });
    const core = { info: jest.fn(), setFailed: jest.fn() };

    await expect(handler({ github, core, owner: 'owner', repo: 'repo', pullRequestNumber: '#1' }))
      .rejects.toThrow('pull-request-number must be a positive integer');
    expect(github.graphql).not.toHaveBeenCalled();
  });

  test('fails the step when the policy cannot be applied', async () => {
    const github = makeOctokit({ pullRequests: [] });
    github.graphql.mockRejectedValue(new Error('rate limited'));
    const core = { info: jest.fn(), setFailed: jest.fn() };

    await expect(handler({ github, core, owner: 'owner', repo: 'repo' })).rejects.toThrow('rate limited');
    expect(core.setFailed).toHaveBeenCalledWith('Error applying the linked issue policy: rate limited');
  });
});
