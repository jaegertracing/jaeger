// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * Unit tests for the duplicate PR labelling
 */

const prDuplicates = require('./pr-duplicates');

describe('processIssueLimitForPullRequest', () => {
  const {
    processIssueLimitForPullRequest,
    processIssueLimitForIssue,
    processAllIssueLimits,
    issueLimitComment,
    issueLimitResolvedComment,
    DUPLICATE_LABEL_NAME,
    OVERRIDE_LABEL_NAME,
    ISSUE_LIMIT_COMMENT_MARKER
  } = prDuplicates;

  const makePR = (number, createdAt, labels = [], isDraft = false, authorAssociation = 'CONTRIBUTOR', linkedIssues) => ({
    number,
    state: 'OPEN',
    isDraft,
    authorAssociation,
    createdAt,
    repository: { nameWithOwner: 'owner/repo' },
    labels: { nodes: labels.map(name => ({ name })) },
    closingIssuesReferences: {
      nodes: linkedIssues || [{
        number: 99,
        repository: { nameWithOwner: 'owner/repo' },
        labels: { nodes: [] }
      }]
    }
  });

  function issueLimitOctokit({ issues, comments = {} }) {
    return {
      graphql: jest.fn().mockResolvedValue({
        repository: { pullRequests: { nodes: issues } }
      }),
      rest: {
        issues: {
          get: jest.fn().mockResolvedValue({ data: { labels: [] } }),
          getLabel: jest.fn().mockResolvedValue({}),
          createLabel: jest.fn().mockResolvedValue({}),
          listComments: jest.fn(({ issue_number }) => Promise.resolve({
            data: (comments[issue_number] || []).map(comment => ({
              user: { login: 'github-actions[bot]' },
              ...comment
            }))
          })),
          addLabels: jest.fn().mockResolvedValue({}),
          removeLabel: jest.fn().mockResolvedValue({}),
          createComment: jest.fn().mockResolvedValue({}),
          updateComment: jest.fn().mockResolvedValue({}),
          deleteComment: jest.fn().mockResolvedValue({})
        }
      }
    };
  }

  test('labels every non-primary active PR, including drafts', async () => {
    const octokit = issueLimitOctokit({
      issues: [
        makePR(30, '2026-01-03T00:00:00Z', [], true),
        makePR(20, '2026-01-02T00:00:00Z'),
        makePR(10, '2026-01-01T00:00:00Z')
      ]
    });

    const result = await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 30);

    expect(result.duplicates).toEqual([20, 30]);
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({
      issue_number: 20, labels: [DUPLICATE_LABEL_NAME]
    }));
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({
      issue_number: 30, labels: [DUPLICATE_LABEL_NAME]
    }));
    expect(octokit.rest.issues.createComment).toHaveBeenCalledTimes(2);
  });

  test('selects the oldest PR and then the lowest number as the primary', async () => {
    const octokit = issueLimitOctokit({
      issues: [
        makePR(20, '2026-01-01T00:00:00Z'),
        makePR(10, '2026-01-01T00:00:00Z')
      ]
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({ issue_number: 20 }));
    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalledWith(expect.objectContaining({ issue_number: 10 }));
  });

  test('counts a maintainer PR instead of exempting it', async () => {
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', [], false, 'MEMBER'),
        makePR(20, '2026-01-02T00:00:00Z')
      ]
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({ issue_number: 20 }));
  });

  test('does not let a stale PR block an active PR and clears its bot marker', async () => {
    const marker = issueLimitComment([{ issueNumber: 99, primaryNumber: 10 }]);
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', ['stale']),
        makePR(20, '2026-01-02T00:00:00Z', ['duplicate'])
      ],
      comments: { 20: [{ id: 7, body: marker }] }
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.removeLabel).toHaveBeenCalledWith(expect.objectContaining({
      issue_number: 20, name: DUPLICATE_LABEL_NAME
    }));
    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(expect.objectContaining({
      comment_id: 7,
      body: issueLimitResolvedComment()
    }));
  });

  test('does not mutate policy state on a stale duplicate PR', async () => {
    const marker = issueLimitComment([{ issueNumber: 99, primaryNumber: 10 }]);
    const octokit = issueLimitOctokit({
      issues: [makePR(20, '2026-01-02T00:00:00Z', ['duplicate', 'stale'])],
      comments: { 20: [{ id: 7, body: marker }] }
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalled();
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();
  });

  test('preserves intentionally competing implementations when the issue is overridden', async () => {
    const override = [{
      number: 99,
      repository: { nameWithOwner: 'owner/repo' },
      labels: { nodes: [{ name: OVERRIDE_LABEL_NAME }] }
    }];
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', override),
        makePR(20, '2026-01-02T00:00:00Z', [], false, 'CONTRIBUTOR', override)
      ]
    });

    const result = await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(result.duplicates).toEqual([]);
    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalled();
  });

  test('updates one existing marker instead of adding another comment on a repeat run', async () => {
    const octokit = issueLimitOctokit({
      issues: [makePR(10, '2026-01-01T00:00:00Z'), makePR(20, '2026-01-02T00:00:00Z', ['duplicate'])],
      comments: { 20: [{ id: 7, body: '<!-- jaeger-pr-per-issue-limit -->\noutdated' }] }
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(expect.objectContaining({ comment_id: 7 }));
  });

  test('is idempotent when labels and the maintained comment already match', async () => {
    const octokit = issueLimitOctokit({
      issues: [makePR(10, '2026-01-01T00:00:00Z'), makePR(20, '2026-01-02T00:00:00Z', ['duplicate'])],
      comments: { 20: [{ id: 7, body: issueLimitComment([{ issueNumber: 99, primaryNumber: 10 }]) }] }
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalled();
    expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();
  });

  test('does not change labels or comments if canonical-link discovery fails', async () => {
    const octokit = issueLimitOctokit({ issues: [] });
    octokit.graphql.mockRejectedValue(new Error('GitHub API unavailable'));

    await expect(processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20)).rejects.toThrow('GitHub API unavailable');

    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalled();
    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalled();
    expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
    expect(octokit.rest.issues.deleteComment).not.toHaveBeenCalled();
  });

  test('sweeps open PRs to reconcile a duplicate after its former primary unlinks the issue', async () => {
    const octokit = issueLimitOctokit({
      issues: [makePR(20, '2026-01-02T00:00:00Z', ['duplicate'])],
      comments: { 20: [{ id: 7, body: issueLimitComment([{ issueNumber: 99, primaryNumber: 10 }]) }] }
    });
    await processAllIssueLimits(octokit, 'owner', 'repo');

    expect(octokit.rest.issues.removeLabel).toHaveBeenCalledWith(expect.objectContaining({
      issue_number: 20,
      name: DUPLICATE_LABEL_NAME
    }));
    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(expect.objectContaining({
      comment_id: 7,
      body: issueLimitResolvedComment()
    }));
  });

  test('does not list comments for unrelated PRs during pull request reconciliation', async () => {
    const issueOne = [{ number: 1, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const issueTwo = [{ number: 2, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const issueThree = [{ number: 3, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', issueOne),
        makePR(20, '2026-01-02T00:00:00Z', [], false, 'CONTRIBUTOR', issueOne),
        makePR(30, '2026-01-03T00:00:00Z', [], false, 'CONTRIBUTOR', issueTwo),
        makePR(40, '2026-01-04T00:00:00Z', ['duplicate'], false, 'CONTRIBUTOR', issueThree)
      ]
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    const listedPRs = octokit.rest.issues.listComments.mock.calls.map(([{ issue_number }]) => issue_number);
    expect(listedPRs).toEqual(expect.arrayContaining([10, 20, 40]));
    expect(listedPRs).not.toContain(30);
  });

  test('reconciles calculated duplicates outside the triggering PRs issues', async () => {
    const issueOne = [{ number: 1, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const issueTwo = [{ number: 2, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const issueThree = [{ number: 3, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', issueOne),
        makePR(20, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', issueTwo),
        makePR(30, '2026-01-02T00:00:00Z', [], false, 'CONTRIBUTOR', issueTwo),
        makePR(40, '2026-01-03T00:00:00Z', [], false, 'CONTRIBUTOR', issueThree)
      ]
    });

    const result = await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 10);

    const listedPRs = octokit.rest.issues.listComments.mock.calls.map(([{ issue_number }]) => issue_number);
    expect(listedPRs).toEqual(expect.arrayContaining([10, 30]));
    expect(listedPRs).not.toContain(40);
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({
      issue_number: 30,
      labels: [DUPLICATE_LABEL_NAME]
    }));
    expect(result.reconciledIssues).toEqual([1, 2]);
  });

  test('does not list comments for unrelated PRs during issue-label reconciliation', async () => {
    const issueOne = [{ number: 1, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const issueTwo = [{ number: 2, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', issueOne),
        makePR(20, '2026-01-02T00:00:00Z', [], false, 'CONTRIBUTOR', issueOne),
        makePR(30, '2026-01-03T00:00:00Z', [], false, 'CONTRIBUTOR', issueTwo)
      ]
    });

    await processIssueLimitForIssue(octokit, 'owner', 'repo', 1);

    const listedPRs = octokit.rest.issues.listComments.mock.calls.map(([{ issue_number }]) => issue_number);
    expect(listedPRs).toEqual(expect.arrayContaining([10, 20]));
    expect(listedPRs).not.toContain(30);
  });

  test('ignores canonical links to issues in other repositories', async () => {
    const foreignIssue = [{
      number: 99,
      repository: { nameWithOwner: 'other/repo' },
      labels: { nodes: [] }
    }];
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', foreignIssue),
        makePR(20, '2026-01-02T00:00:00Z', [], false, 'CONTRIBUTOR', foreignIssue)
      ]
    });

    const result = await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(result.duplicates).toEqual([]);
    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalled();
    const listedPRs = octokit.rest.issues.listComments.mock.calls.map(([{ issue_number }]) => issue_number);
    expect(listedPRs).not.toContain(10);
  });

  test('paginates the open PR graph before selecting a primary', async () => {
    const octokit = issueLimitOctokit({ issues: [] });
    octokit.graphql
      .mockResolvedValueOnce({
        repository: {
          pullRequests: {
            nodes: [makePR(10, '2026-01-01T00:00:00Z')],
            pageInfo: { hasNextPage: true, endCursor: 'next-page' }
          }
        }
      })
      .mockResolvedValueOnce({
        repository: {
          pullRequests: {
            nodes: [makePR(20, '2026-01-02T00:00:00Z')],
            pageInfo: { hasNextPage: false, endCursor: null }
          }
        }
      });

    await processAllIssueLimits(octokit, 'owner', 'repo');

    expect(octokit.graphql).toHaveBeenNthCalledWith(1, expect.any(String), {
      owner: 'owner', repo: 'repo', after: null
    });
    expect(octokit.graphql).toHaveBeenNthCalledWith(2, expect.any(String), {
      owner: 'owner', repo: 'repo', after: 'next-page'
    });
    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({ issue_number: 20 }));
  });

  test('restores earlier duplicate labels if a later cleanup removal fails', async () => {
    const override = [{
      number: 99,
      repository: { nameWithOwner: 'owner/repo' },
      labels: { nodes: [{ name: OVERRIDE_LABEL_NAME }] }
    }];
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', ['duplicate'], false, 'CONTRIBUTOR', override),
        makePR(20, '2026-01-02T00:00:00Z', ['duplicate'], false, 'CONTRIBUTOR', override)
      ],
      comments: {
        10: [{ id: 7, body: issueLimitComment([{ issueNumber: 99, primaryNumber: 1 }]) }],
        20: [{ id: 8, body: issueLimitComment([{ issueNumber: 99, primaryNumber: 1 }]) }]
      }
    });
    octokit.rest.issues.removeLabel
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error('GitHub API unavailable'));

    await expect(processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20)).rejects.toThrow(
      'Failed to remove duplicate label from PR #20'
    );

    expect(octokit.rest.issues.addLabels).toHaveBeenCalledWith(expect.objectContaining({
      issue_number: 10,
      labels: [DUPLICATE_LABEL_NAME]
    }));
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalled();

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(expect.objectContaining({
      comment_id: 7,
      body: issueLimitResolvedComment()
    }));
  });

  test('keeps a PR duplicate for one issue when it is primary for another', async () => {
    const issueOne = [{ number: 1, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const issueTwo = [{ number: 2, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }];
    const bothIssues = [...issueOne, ...issueTwo];
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', issueOne),
        makePR(20, '2026-01-02T00:00:00Z', ['duplicate'], false, 'CONTRIBUTOR', bothIssues),
        makePR(30, '2026-01-03T00:00:00Z', [], false, 'CONTRIBUTOR', issueTwo)
      ],
      comments: { 20: [{ id: 7, body: issueLimitComment([{ issueNumber: 1, primaryNumber: 10 }]) }] }
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 30);

    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalledWith(expect.objectContaining({ issue_number: 20 }));
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalledWith(expect.objectContaining({
      comment_id: 7,
      body: issueLimitResolvedComment()
    }));
    expect(octokit.rest.issues.createComment).toHaveBeenCalledWith(expect.objectContaining({
      issue_number: 30,
      body: issueLimitComment([{ issueNumber: 2, primaryNumber: 20 }])
    }));
  });

  test('does not trust a marker quoted by a human when managing duplicate labels', async () => {
    const octokit = issueLimitOctokit({
      issues: [makePR(10, '2026-01-01T00:00:00Z'), makePR(20, '2026-01-02T00:00:00Z', ['duplicate'])],
      comments: { 20: [{ id: 7, user: { login: 'maintainer' }, body: issueLimitComment([{ issueNumber: 99, primaryNumber: 10 }]) }] }
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.createComment).toHaveBeenCalledTimes(1);
    expect(octokit.rest.issues.updateComment).not.toHaveBeenCalledWith(expect.objectContaining({ comment_id: 7 }));
  });

  test('does not remove a manually applied duplicate label', async () => {
    const octokit = issueLimitOctokit({
      issues: [makePR(20, '2026-01-02T00:00:00Z', ['duplicate'])],
      comments: { 20: [{ id: 7, user: { login: 'maintainer' }, body: ISSUE_LIMIT_COMMENT_MARKER }] }
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });

  test('clears an ownership marker when adding its duplicate label fails', async () => {
    const comments = { 20: [] };
    const octokit = issueLimitOctokit({
      issues: [makePR(10, '2026-01-01T00:00:00Z'), makePR(20, '2026-01-02T00:00:00Z')],
      comments
    });
    octokit.rest.issues.createComment.mockImplementation(({ issue_number, body }) => {
      comments[issue_number].push({ id: 7, body });
      return Promise.resolve({});
    });
    octokit.rest.issues.addLabels.mockRejectedValueOnce(new Error('GitHub API unavailable'));

    await expect(processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20)).rejects.toThrow(
      'Failed to add duplicate label to PR #20'
    );
    expect(comments[20][0].body).toContain('label-owned');

    octokit.graphql.mockResolvedValue({
      repository: { pullRequests: { nodes: [makePR(20, '2026-01-02T00:00:00Z', [], false, 'CONTRIBUTOR', [])] } }
    });
    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(expect.objectContaining({
      comment_id: 7,
      body: issueLimitResolvedComment()
    }));
  });

  test('does not claim or remove a pre-existing duplicate label', async () => {
    const comments = { 20: [] };
    const octokit = issueLimitOctokit({
      issues: [
        makePR(10, '2026-01-01T00:00:00Z'),
        makePR(20, '2026-01-02T00:00:00Z', ['duplicate'])
      ],
      comments
    });
    octokit.rest.issues.createComment.mockImplementation(({ issue_number, body }) => {
      comments[issue_number].push({ id: 7, body });
      return Promise.resolve({});
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(comments[20][0].body).not.toContain('label-owned');
    octokit.graphql.mockResolvedValue({
      repository: { pullRequests: { nodes: [makePR(20, '2026-01-02T00:00:00Z', ['duplicate'], false, 'CONTRIBUTOR', [])] } }
    });
    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });

  test('retries a resolved policy comment update after it fails', async () => {
    const comments = {
      20: [{ id: 7, body: issueLimitComment([{ issueNumber: 99, primaryNumber: 10 }]) }]
    };
    const octokit = issueLimitOctokit({
      issues: [makePR(20, '2026-01-02T00:00:00Z', ['duplicate'], false, 'CONTRIBUTOR', [])],
      comments
    });
    octokit.rest.issues.updateComment.mockRejectedValueOnce(new Error('GitHub API unavailable'));

    await expect(processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20)).rejects.toThrow(
      'GitHub API unavailable'
    );

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(expect.objectContaining({
      comment_id: 7,
      body: issueLimitResolvedComment()
    }));
  });

  test('reuses its policy comment when a resolved PR becomes duplicate again', async () => {
    const comments = { 20: [] };
    const octokit = issueLimitOctokit({
      issues: [makePR(10, '2026-01-01T00:00:00Z'), makePR(20, '2026-01-02T00:00:00Z')],
      comments
    });
    octokit.rest.issues.createComment.mockImplementation(({ issue_number, body }) => {
      comments[issue_number].push({ id: 7, body });
      return Promise.resolve({});
    });
    octokit.rest.issues.updateComment.mockImplementation(({ comment_id, body }) => {
      comments[20].find(comment => comment.id === comment_id).body = body;
      return Promise.resolve({});
    });

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    const override = [{
      number: 99,
      repository: { nameWithOwner: 'owner/repo' },
      labels: { nodes: [{ name: OVERRIDE_LABEL_NAME }] }
    }];
    octokit.graphql.mockResolvedValue({
      repository: {
        pullRequests: {
          nodes: [
            makePR(10, '2026-01-01T00:00:00Z', [], false, 'CONTRIBUTOR', override),
            makePR(20, '2026-01-02T00:00:00Z', ['duplicate'], false, 'CONTRIBUTOR', override)
          ]
        }
      }
    });
    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);
    expect(comments[20][0].body).toBe(issueLimitResolvedComment());

    octokit.graphql.mockResolvedValue({
      repository: {
        pullRequests: {
          nodes: [makePR(10, '2026-01-01T00:00:00Z'), makePR(20, '2026-01-02T00:00:00Z')]
        }
      }
    });
    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20);

    expect(octokit.rest.issues.createComment).toHaveBeenCalledTimes(1);
    expect(comments[20][0].body).toBe(issueLimitComment([{ issueNumber: 99, primaryNumber: 10 }]));
  });

  test('bounds concurrent comment reads during an open PR sweep', async () => {
    const openPRs = Array.from({ length: 12 }, (_, index) =>
      makePR(index + 1, `2026-01-${String(index + 1).padStart(2, '0')}T00:00:00Z`)
    );
    const octokit = issueLimitOctokit({ issues: openPRs });
    let activeRequests = 0;
    let maximumActiveRequests = 0;
    octokit.rest.issues.listComments.mockImplementation(async () => {
      activeRequests++;
      maximumActiveRequests = Math.max(maximumActiveRequests, activeRequests);
      await new Promise(resolve => setImmediate(resolve));
      activeRequests--;
      return { data: [] };
    });

    await processAllIssueLimits(octokit, 'owner', 'repo');

    expect(maximumActiveRequests).toBeLessThanOrEqual(10);
    expect(octokit.rest.issues.listComments).toHaveBeenCalledTimes(openPRs.length);
  });

  test('fails safely when a nested connection is truncated', async () => {
    const octokit = issueLimitOctokit({ issues: [] });
    octokit.graphql.mockResolvedValue({
      repository: {
        pullRequests: {
          nodes: [{
            number: 20,
            createdAt: '2026-01-02T00:00:00Z',
            labels: { nodes: [], pageInfo: { hasNextPage: false } },
            closingIssuesReferences: { nodes: [], pageInfo: { hasNextPage: true } }
          }]
        }
      }
    });

    await expect(processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20)).rejects.toThrow(
      'PR #20 has more linked issues than the per-issue policy query supports'
    );
    expect(octokit.rest.issues.addLabels).not.toHaveBeenCalled();
    expect(octokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });

  test('fails safely when PR or issue labels exceed the query limits', async () => {
    const octokit = issueLimitOctokit({ issues: [] });
    octokit.graphql.mockResolvedValue({
      repository: {
        pullRequests: {
          nodes: [{
            number: 20,
            createdAt: '2026-01-02T00:00:00Z',
            labels: { nodes: [], pageInfo: { hasNextPage: true } },
            closingIssuesReferences: { nodes: [], pageInfo: { hasNextPage: false } }
          }]
        }
      }
    });

    await expect(processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20)).rejects.toThrow(
      'PR #20 has more labels than the per-issue policy query supports'
    );

    octokit.graphql.mockResolvedValue({
      repository: {
        pullRequests: {
          nodes: [{
            number: 20,
            createdAt: '2026-01-02T00:00:00Z',
            labels: { nodes: [], pageInfo: { hasNextPage: false } },
            closingIssuesReferences: {
              nodes: [{
                number: 99,
                repository: { nameWithOwner: 'owner/repo' },
                labels: { nodes: [], pageInfo: { hasNextPage: true } }
              }],
              pageInfo: { hasNextPage: false }
            }
          }]
        }
      }
    });

    await expect(processIssueLimitForPullRequest(octokit, 'owner', 'repo', 20)).rejects.toThrow(
      'Issue #99 has more labels than the per-issue policy query supports'
    );
  });
});

describe('githubActionHandler', () => {
  test('runs the full sweep without a pull request or issue number', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      graphql: jest.fn().mockResolvedValue({
        repository: { pullRequests: { nodes: [] } }
      }),
      rest: { issues: {} }
    };

    await prDuplicates({
      github,
      core,
      owner: 'owner',
      repo: 'repo',
    });

    expect(core.setFailed).not.toHaveBeenCalled();
    expect(core.info).toHaveBeenCalledWith('Reconciled linked issues: none');
    expect(core.info).toHaveBeenCalledWith('Duplicate PRs: none');
  });

  test('logs the manual sweep plan in dry-run mode', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      graphql: jest.fn().mockResolvedValue({
        repository: { pullRequests: { nodes: [] } }
      }),
      rest: { issues: {} }
    };

    await prDuplicates({
      github,
      core,
      owner: 'owner',
      repo: 'repo',
      dryRun: true
    });

    expect(core.info).toHaveBeenCalledWith('Dry-run plan linked issues: none');
    expect(core.info).toHaveBeenCalledWith('Dry-run plan duplicate PRs: none');
  });
  // PR 20 closes issue 99 and PR 30 closes issue 77, so neither is a duplicate
  // and a run differs only in whose comments it looks up.
  function githubWithTwoUnrelatedPRs() {
    const pr = (number, issue) => ({
      number,
      state: 'OPEN',
      createdAt: '2026-01-01T00:00:00Z',
      labels: { nodes: [] },
      closingIssuesReferences: {
        nodes: [{ number: issue, repository: { nameWithOwner: 'owner/repo' }, labels: { nodes: [] } }]
      }
    });
    return {
      graphql: jest.fn().mockResolvedValue({
        repository: { pullRequests: { nodes: [pr(20, 99), pr(30, 77)] } }
      }),
      rest: { issues: { listComments: jest.fn().mockResolvedValue({ data: [] }) } }
    };
  }

  const commentLookups = github =>
    github.rest.issues.listComments.mock.calls.map(([args]) => args.issue_number).sort();

  test('looks up the comments of the pull request of an event and of no other', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = githubWithTwoUnrelatedPRs();

    await prDuplicates({ github, core, owner: 'owner', repo: 'repo', pullRequestNumber: '20' });

    expect(core.setFailed).not.toHaveBeenCalled();
    expect(commentLookups(github)).toEqual([20]);
  });

  test('looks up the comments of the pull requests linked to the issue of a label event', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = githubWithTwoUnrelatedPRs();

    await prDuplicates({ github, core, owner: 'owner', repo: 'repo', issueNumber: '77' });

    expect(core.setFailed).not.toHaveBeenCalled();
    expect(commentLookups(github)).toEqual([30]);
  });

  test('looks up the comments of every pull request without a number', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = githubWithTwoUnrelatedPRs();

    await prDuplicates({ github, core, owner: 'owner', repo: 'repo' });

    expect(commentLookups(github)).toEqual([20, 30]);
  });

  test('reports a failed query and rethrows it', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = { graphql: jest.fn().mockRejectedValue(new Error('boom')), rest: { issues: {} } };

    await expect(prDuplicates({ github, core, owner: 'owner', repo: 'repo' })).rejects.toThrow('boom');

    expect(core.setFailed).toHaveBeenCalledWith('Error labelling duplicate pull requests: boom');
  });
});
