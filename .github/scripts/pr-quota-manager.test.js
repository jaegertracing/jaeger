// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * Unit tests for PR Quota Management System
 */

const prQuotaManager = require('./pr-quota-manager');
const {
  formatStatus,
  calculateQuota,
  fetchAuthorPRs,
  processQuotaForAuthor,
  ensureLabelExists,
  addLabel,
  removeLabel,
  hasBlockingComment,
  postBlockingComment,
  postUnblockingComment,
  LABEL_NAME,
  LABEL_COLOR
} = prQuotaManager;

// Mock logger to suppress output during tests
const mockLogger = {
  log: jest.fn(),
  error: jest.fn()
};

describe('formatStatus', () => {
  test('formats open count and quota as bullet points', () => {
    expect(formatStatus(3, 5)).toBe('  * Open: 3\n  * Limit: 5');
  });
});

describe('calculateQuota', () => {
  test('returns 1 for 0 merged PRs', () => {
    expect(calculateQuota(0)).toBe(1);
  });

  test('returns 2 for 1 merged PR', () => {
    expect(calculateQuota(1)).toBe(2);
  });

  test('returns 3 for 2 merged PRs', () => {
    expect(calculateQuota(2)).toBe(3);
  });

  test('returns 10 (unlimited) for 3 merged PRs', () => {
    expect(calculateQuota(3)).toBe(10);
  });

  test('returns 10 (unlimited) for 10 merged PRs', () => {
    expect(calculateQuota(10)).toBe(10);
  });
});

describe('fetchAuthorPRs', () => {
  test('fetches open PRs and merged count', async () => {
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn()
            // First call for open PRs
            .mockResolvedValueOnce({
              data: [
                { number: 1, user: { login: 'testuser' }, state: 'open', merged_at: null },
                { number: 2, user: { login: 'otheruser' }, state: 'open', merged_at: null },
                { number: 3, user: { login: 'testuser' }, state: 'open', merged_at: null }
              ]
            })
            // Second call for closed/merged PRs
            .mockResolvedValueOnce({
              data: [
                { number: 10, user: { login: 'testuser' }, merged_at: '2024-01-01' },
                { number: 11, user: { login: 'otheruser' }, merged_at: '2024-01-02' }
              ]
            })
        }
      }
    };

    const result = await fetchAuthorPRs(mockOctokit, 'owner', 'repo', 'testuser');

    expect(result.openPRs).toHaveLength(2);
    expect(result.openPRs[0].number).toBe(1);
    expect(result.openPRs[1].number).toBe(3);
    expect(result.mergedCount).toBe(1);
  });

  test('stops fetching merged PRs after finding 3', async () => {
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn()
            // Open PRs call
            .mockResolvedValueOnce({ data: [] })
            // First batch of closed PRs with 3 merged
            .mockResolvedValueOnce({
              data: [
                { number: 1, user: { login: 'testuser' }, merged_at: '2024-01-01' },
                { number: 2, user: { login: 'testuser' }, merged_at: '2024-01-02' },
                { number: 3, user: { login: 'testuser' }, merged_at: '2024-01-03' },
                { number: 4, user: { login: 'testuser' }, merged_at: null }, // closed but not merged
              ]
            })
        }
      }
    };

    const result = await fetchAuthorPRs(mockOctokit, 'owner', 'repo', 'testuser');

    expect(result.mergedCount).toBe(3);
    // Should stop after finding 3 merged PRs, so only 2 calls (1 for open, 1 for closed)
    expect(mockOctokit.rest.pulls.list).toHaveBeenCalledTimes(2);
  });
});

describe('ensureLabelExists', () => {
  test('does not create label if it already exists', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          getLabel: jest.fn().mockResolvedValue({ data: { name: LABEL_NAME } }),
          createLabel: jest.fn()
        }
      }
    };

    await ensureLabelExists(mockOctokit, 'owner', 'repo', mockLogger);

    expect(mockOctokit.rest.issues.getLabel).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      name: LABEL_NAME
    });
    expect(mockOctokit.rest.issues.createLabel).not.toHaveBeenCalled();
  });

  test('creates label if it does not exist', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          getLabel: jest.fn().mockRejectedValue({ status: 404 }),
          createLabel: jest.fn().mockResolvedValue({})
        }
      }
    };

    await ensureLabelExists(mockOctokit, 'owner', 'repo', mockLogger);

    expect(mockOctokit.rest.issues.createLabel).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      name: LABEL_NAME,
      color: LABEL_COLOR,
      description: 'PR is on hold due to quota limits for new contributors'
    });
  });
});

describe('addLabel', () => {
  test('adds label to PR', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          addLabels: jest.fn().mockResolvedValue({})
        }
      }
    };

    await addLabel(mockOctokit, 'owner', 'repo', 123, mockLogger);

    expect(mockOctokit.rest.issues.addLabels).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      issue_number: 123,
      labels: [LABEL_NAME]
    });
  });

  test('throws when the API rejects, so the job fails instead of silently skipping', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          addLabels: jest.fn().mockRejectedValue(
            new Error('Resource not accessible by personal access token')
          )
        }
      }
    };

    await expect(addLabel(mockOctokit, 'owner', 'repo', 123, mockLogger)).rejects.toThrow(
      'Failed to add label to PR #123: Resource not accessible by personal access token'
    );
  });
});

describe('removeLabel', () => {
  test('removes label from PR', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          removeLabel: jest.fn().mockResolvedValue({})
        }
      }
    };

    await removeLabel(mockOctokit, 'owner', 'repo', 123, mockLogger);

    expect(mockOctokit.rest.issues.removeLabel).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      issue_number: 123,
      name: LABEL_NAME
    });
  });

  test('ignores 404 errors when label is not present', async () => {
    const testLogger = {
      log: jest.fn(),
      error: jest.fn()
    };

    const mockOctokit = {
      rest: {
        issues: {
          removeLabel: jest.fn().mockRejectedValue({ status: 404 })
        }
      }
    };

    await expect(
      removeLabel(mockOctokit, 'owner', 'repo', 123, testLogger)
    ).resolves.toBeUndefined();

    expect(testLogger.error).not.toHaveBeenCalled();
  });

  test('throws on non-404 errors', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          removeLabel: jest.fn().mockRejectedValue({ status: 500, message: 'Server error' })
        }
      }
    };

    await expect(removeLabel(mockOctokit, 'owner', 'repo', 123, mockLogger)).rejects.toThrow(
      'Failed to remove label from PR #123: Server error'
    );
  });
});

describe('hasBlockingComment', () => {
  test('returns true if blocking comment exists', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          listComments: jest.fn().mockResolvedValue({
            data: [
              { body: 'Some other comment' },
              { body: 'This PR is currently **on hold**' }
            ]
          })
        }
      }
    };

    const result = await hasBlockingComment(mockOctokit, 'owner', 'repo', 123);

    expect(result).toBe(true);
  });

  test('returns false if blocking comment does not exist', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          listComments: jest.fn().mockResolvedValue({
            data: [
              { body: 'Some other comment' },
              { body: 'Another comment' }
            ]
          })
        }
      }
    };

    const result = await hasBlockingComment(mockOctokit, 'owner', 'repo', 123);

    expect(result).toBe(false);
  });
});



describe('postBlockingComment', () => {
  test('posts blocking comment if none exists', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          listComments: jest.fn().mockResolvedValue({ data: [] }),
          createComment: jest.fn().mockResolvedValue({})
        }
      }
    };

    await postBlockingComment(mockOctokit, 'owner', 'repo', 123, 'testuser', 2, 1, mockLogger);

    expect(mockOctokit.rest.issues.createComment).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      issue_number: 123,
      body: expect.stringContaining('This PR is currently **on hold**')
    });
  });

  test('skips comment if blocking comment already exists', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          listComments: jest.fn().mockResolvedValue({
            data: [{ body: 'This PR is currently **on hold**' }]
          }),
          createComment: jest.fn()
        }
      }
    };

    await postBlockingComment(mockOctokit, 'owner', 'repo', 123, 'testuser', 2, 1, mockLogger);

    expect(mockOctokit.rest.issues.createComment).not.toHaveBeenCalled();
  });

  test('throws when the API rejects', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          listComments: jest.fn().mockResolvedValue({ data: [] }),
          createComment: jest.fn().mockRejectedValue(
            new Error('Resource not accessible by personal access token')
          )
        }
      }
    };

    await expect(
      postBlockingComment(mockOctokit, 'owner', 'repo', 123, 'testuser', 2, 1, mockLogger)
    ).rejects.toThrow('Failed to post blocking comment on PR #123');
  });
});

describe('postUnblockingComment', () => {
  test('always posts unblocking comment', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          createComment: jest.fn().mockResolvedValue({})
        }
      }
    };

    await postUnblockingComment(mockOctokit, 'owner', 'repo', 123, 'testuser', 1, 2, mockLogger);

    expect(mockOctokit.rest.issues.createComment).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      issue_number: 123,
      body: expect.stringContaining('PR quota unlocked!')
    });
  });

  test('throws when the API rejects', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          createComment: jest.fn().mockRejectedValue(new Error('Server error'))
        }
      }
    };

    await expect(
      postUnblockingComment(mockOctokit, 'owner', 'repo', 123, 'testuser', 1, 2, mockLogger)
    ).rejects.toThrow('Failed to post unblocking comment on PR #123');
  });
});

describe('processQuotaForAuthor', () => {
  test('blocks PRs exceeding quota for new contributor', async () => {
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn()
            // Open PRs call
            .mockResolvedValueOnce({
              data: [
                {
                  number: 1,
                  user: { login: 'newuser' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-01T00:00:00Z',
                  labels: []
                },
                {
                  number: 2,
                  user: { login: 'newuser' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-02T00:00:00Z',
                  labels: []
                }
              ]
            })
            // Closed PRs call (no merged PRs found)
            .mockResolvedValueOnce({ data: [] })
        },
        issues: {
          getLabel: jest.fn().mockResolvedValue({ data: { name: LABEL_NAME } }),
          addLabels: jest.fn().mockResolvedValue({}),
          listComments: jest.fn().mockResolvedValue({ data: [] }),
          createComment: jest.fn().mockResolvedValue({})
        }
      }
    };

    const result = await processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'newuser', mockLogger);

    expect(result.mergedCount).toBe(0);
    expect(result.quota).toBe(1);
    expect(result.openCount).toBe(2);
    expect(result.results.blocked).toEqual([2]);
    expect(result.results.unchanged).toEqual([1]);
  });

  // Regression test: the label/comment writes used to be caught and logged, so a token
  // without Issues write permission produced a successful run that reported every PR as
  // blocked while nothing was labelled.
  test('fails the run instead of reporting success when labelling is not permitted', async () => {
    const testLogger = {
      log: jest.fn(),
      error: jest.fn()
    };

    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn()
            .mockResolvedValueOnce({
              data: [
                {
                  number: 1,
                  user: { login: 'newuser' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-01T00:00:00Z',
                  labels: []
                },
                {
                  number: 2,
                  user: { login: 'newuser' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-02T00:00:00Z',
                  labels: []
                }
              ]
            })
            .mockResolvedValueOnce({ data: [] })
        },
        issues: {
          getLabel: jest.fn().mockResolvedValue({ data: { name: LABEL_NAME } }),
          addLabels: jest.fn().mockRejectedValue(
            new Error('Resource not accessible by personal access token')
          ),
          listComments: jest.fn().mockResolvedValue({ data: [] }),
          createComment: jest.fn().mockResolvedValue({})
        }
      }
    };

    await expect(
      processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'newuser', testLogger)
    ).rejects.toThrow('Failed to add label to PR #2');

    const logged = testLogger.log.mock.calls.map(call => call.join(' ')).join('\n');
    expect(logged).not.toContain('Labeled PR #2 as blocked');
  });

  test('unblocks PRs when quota becomes available', async () => {
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn()
            // Open PRs call
            .mockResolvedValueOnce({
              data: [
                {
                  number: 1,
                  user: { login: 'contributor' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-01T00:00:00Z',
                  labels: []
                },
                {
                  number: 3,
                  user: { login: 'contributor' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-03T00:00:00Z',
                  labels: [{ name: LABEL_NAME }]
                }
              ]
            })
            // Closed PRs call (1 merged)
            .mockResolvedValueOnce({
              data: [
                {
                  number: 2,
                  user: { login: 'contributor' },
                  merged_at: '2024-01-05T00:00:00Z'
                }
              ]
            })
        },
        issues: {
          getLabel: jest.fn().mockResolvedValue({ data: { name: LABEL_NAME } }),
          removeLabel: jest.fn().mockResolvedValue({}),
          listComments: jest.fn().mockResolvedValue({ data: [] }),
          createComment: jest.fn().mockResolvedValue({})
        }
      }
    };

    const result = await processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'contributor', mockLogger);

    expect(result.mergedCount).toBe(1);
    expect(result.quota).toBe(2);
    expect(result.openCount).toBe(2);
    expect(result.results.unblocked).toEqual([3]);
  });

  test('processes PRs in order by creation date (oldest first)', async () => {
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn()
            // Open PRs are already sorted by creation date from the API
            .mockResolvedValueOnce({
              data: [
                {
                  number: 1,
                  user: { login: 'user' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-01T00:00:00Z',
                  labels: []
                },
                {
                  number: 2,
                  user: { login: 'user' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-02T00:00:00Z',
                  labels: []
                },
                {
                  number: 3,
                  user: { login: 'user' },
                  state: 'open',
                  merged_at: null,
                  created_at: '2024-01-03T00:00:00Z',
                  labels: []
                }
              ]
            })
            // No merged PRs
            .mockResolvedValueOnce({ data: [] })
        },
        issues: {
          getLabel: jest.fn().mockResolvedValue({ data: { name: LABEL_NAME } }),
          addLabels: jest.fn().mockResolvedValue({}),
          listComments: jest.fn().mockResolvedValue({ data: [] }),
          createComment: jest.fn().mockResolvedValue({})
        }
      }
    };

    const result = await processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'user', mockLogger);

    // First PR (oldest) should not be blocked, others should be
    expect(result.results.unchanged).toEqual([1]);
    expect(result.results.blocked).toEqual([2, 3]);
  });
});

describe('processIssueLimitForPullRequest', () => {
  const {
    processIssueLimitForPullRequest,
    processAllIssueLimits,
    issueLimitComment,
    issueLimitResolvedComment,
    DUPLICATE_LABEL_NAME,
    OVERRIDE_LABEL_NAME,
    ISSUE_LIMIT_COMMENT_MARKER
  } = prQuotaManager;

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
    const marker = '<!-- jaeger-pr-per-issue-limit -->\nold';
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

    await processIssueLimitForPullRequest(octokit, 'owner', 'repo', 10);

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
  test('runs the per-issue sweep without a pull request author', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      graphql: jest.fn().mockResolvedValue({
        repository: { pullRequests: { nodes: [] } }
      }),
      rest: { issues: {} }
    };

    await prQuotaManager({
      github,
      core,
      owner: 'owner',
      repo: 'repo',
      perIssueLimit: true
    });

    expect(core.setFailed).not.toHaveBeenCalled();
    expect(core.info).toHaveBeenCalledWith('Reconciled the open pull request graph.');
  });

  test('does not run the author quota when per-issue reconciliation receives a username', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      graphql: jest.fn().mockResolvedValue({ repository: { pullRequests: { nodes: [] } } }),
      rest: { issues: {}, pulls: { list: jest.fn() } }
    };

    await prQuotaManager({
      github,
      core,
      username: 'contributor',
      owner: 'owner',
      repo: 'repo',
      perIssueLimit: true
    });

    expect(github.rest.pulls.list).not.toHaveBeenCalled();
    expect(core.setFailed).not.toHaveBeenCalled();
  });
});
