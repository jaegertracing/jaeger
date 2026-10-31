// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * Unit tests for PR Quota Management System
 */

const prQuotaManager = require('./pr-quota-manager');
const {
  formatStatus,
  isExempt,
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
  LABEL_COLOR,
  UNLIMITED_QUOTA
} = prQuotaManager;

// Search is called twice: merged history across both repos, then closed PRs
// in the current repo that still have the quota label.
function mockQuotaSearch({ mergedCount = 0, closedLabeled = [] } = {}) {
  return jest.fn(async ({ q, page }) => {
    if (q.includes('is:merged')) {
      return { data: { total_count: mergedCount, incomplete_results: false, items: [] } };
    }
    if (q.includes('is:closed')) {
      const start = ((page || 1) - 1) * 100;
      const items = closedLabeled.slice(start, start + 100);
      return {
        data: {
          total_count: closedLabeled.length,
          incomplete_results: false,
          items
        }
      };
    }
    throw new Error(`unexpected search query: ${q}`);
  });
}

// Mock logger to suppress output during tests
const mockLogger = {
  log: jest.fn(),
  error: jest.fn()
};

describe('formatStatus', () => {
  test('formats open count and quota as bullet points', () => {
    expect(formatStatus(3, 5)).toBe('  * Open: 3\n  * Limit: 5');
  });

  test('prints unlimited when there is no cap', () => {
    expect(formatStatus(4, UNLIMITED_QUOTA)).toBe('  * Open: 4\n  * Limit: unlimited');
  });
});

describe('isExempt', () => {
  const original = process.env.QUOTA_EXEMPT_USERS;

  afterEach(() => {
    if (original === undefined) {
      delete process.env.QUOTA_EXEMPT_USERS;
    } else {
      process.env.QUOTA_EXEMPT_USERS = original;
    }
  });

  test('matches a comma-separated list without regard to case or surrounding spaces', () => {
    process.env.QUOTA_EXEMPT_USERS = ' other , NewUser ';
    expect(isExempt('newuser')).toBe(true);
    expect(isExempt('OTHER')).toBe(true);
    expect(isExempt('someone-else')).toBe(false);
  });

  test('splits on whitespace, including newlines, and strips a leading @', () => {
    process.env.QUOTA_EXEMPT_USERS = '@One\nTwo, three';
    expect(isExempt('one')).toBe(true);
    expect(isExempt('@two')).toBe(true);
    expect(isExempt('three')).toBe(true);
    expect(isExempt('four')).toBe(false);
  });

  test('treats a missing or empty variable as no exemptions', () => {
    delete process.env.QUOTA_EXEMPT_USERS;
    expect(isExempt('newuser')).toBe(false);
    process.env.QUOTA_EXEMPT_USERS = ' , ';
    expect(isExempt('newuser')).toBe(false);
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

  test('returns unlimited for 3 merged PRs', () => {
    expect(calculateQuota(3)).toBe(UNLIMITED_QUOTA);
  });

  test('returns unlimited for 10 merged PRs', () => {
    expect(calculateQuota(10)).toBe(UNLIMITED_QUOTA);
  });
});

describe('fetchAuthorPRs', () => {
  test('fetches open PRs in this repo and the shared merged count', async () => {
    const search = mockQuotaSearch({ mergedCount: 1, closedLabeled: [{ number: 50 }] });
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn().mockResolvedValue({
            data: [
              { number: 1, user: { login: 'testuser' }, state: 'open', merged_at: null },
              { number: 2, user: { login: 'otheruser' }, state: 'open', merged_at: null },
              { number: 3, user: { login: 'testuser' }, state: 'open', merged_at: null }
            ]
          })
        },
        search: { issuesAndPullRequests: search }
      }
    };

    const result = await fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger-ui', 'testuser');

    expect(result.openPRs).toHaveLength(2);
    expect(result.openPRs[0].number).toBe(1);
    expect(result.openPRs[1].number).toBe(3);
    expect(result.mergedCount).toBe(1);
    expect(result.closedLabeledPRs).toEqual([{ number: 50 }]);
    expect(mockOctokit.rest.pulls.list).toHaveBeenCalledTimes(1);

    const mergedQuery = search.mock.calls.map(call => call[0].q).find(q => q.includes('is:merged'));
    expect(mergedQuery).toBe(
      'is:pr is:merged author:"testuser" repo:jaegertracing/jaeger repo:jaegertracing/jaeger-ui'
    );
    const closedQuery = search.mock.calls.map(call => call[0].q).find(q => q.includes('is:closed'));
    expect(closedQuery).toBe(
      'repo:jaegertracing/jaeger-ui is:pr is:closed label:"pr-quota-reached" author:"testuser"'
    );
  });

  test('does not cap the merged count at 3', async () => {
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn().mockResolvedValue({ data: [] })
        },
        search: { issuesAndPullRequests: mockQuotaSearch({ mergedCount: 5 }) }
      }
    };

    const result = await fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'testuser');

    expect(result.mergedCount).toBe(5);
    expect(mockOctokit.rest.pulls.list).toHaveBeenCalledTimes(1);
  });

  test('pages through closed quota labels', async () => {
    const closedLabeled = Array.from({ length: 101 }, (_, index) => ({ number: index + 1 }));
    const search = mockQuotaSearch({ mergedCount: 0, closedLabeled });
    const mockOctokit = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: { issuesAndPullRequests: search }
      }
    };

    const result = await fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'testuser');

    expect(result.closedLabeledPRs).toHaveLength(101);
    const closedPages = search.mock.calls
      .map(call => call[0])
      .filter(args => args.q.includes('is:closed'))
      .map(args => args.page);
    expect(closedPages).toEqual([1, 2]);
  });

  test('rejects when search results are incomplete', async () => {
    const mockOctokit = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: {
          issuesAndPullRequests: jest.fn().mockResolvedValue({
            data: { total_count: 0, incomplete_results: true, items: [] }
          })
        }
      }
    };

    await expect(fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'testuser'))
      .rejects.toThrow('incomplete results');
  });

  test('rejects when search omits total_count', async () => {
    const mockOctokit = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: {
          issuesAndPullRequests: jest.fn().mockResolvedValue({
            data: { incomplete_results: false, items: [] }
          })
        }
      }
    };

    await expect(fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'testuser'))
      .rejects.toThrow('no total_count');
  });

  test('follows a second page of open pull requests', async () => {
    const page = Array.from({ length: 100 }, (_, index) => ({
      number: index + 1,
      user: { login: index === 0 ? 'testuser' : 'otheruser' }
    }));
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn()
            .mockResolvedValueOnce({ data: page })
            .mockResolvedValueOnce({
              data: [{ number: 101, user: { login: 'testuser' } }]
            })
        },
        search: { issuesAndPullRequests: mockQuotaSearch({ mergedCount: 0 }) }
      }
    };

    const result = await fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'testuser');

    expect(result.openPRs.map(pr => pr.number)).toEqual([1, 101]);
    expect(mockOctokit.rest.pulls.list).toHaveBeenCalledTimes(2);
  });

  test('rejects a closed-label search that stops short of its total', async () => {
    const fullPage = Array.from({ length: 100 }, (_, index) => ({ number: index + 1 }));
    const search = jest.fn(async ({ q, page }) => {
      if (q.includes('is:merged')) {
        return { data: { total_count: 0, incomplete_results: false, items: [] } };
      }
      if (page === 1) {
        return { data: { total_count: 250, incomplete_results: false, items: fullPage } };
      }
      return { data: { total_count: 250, incomplete_results: false, items: [{ number: 101 }] } };
    });
    const mockOctokit = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: { issuesAndPullRequests: search }
      }
    };

    await expect(fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'testuser'))
      .rejects.toThrow('returned 101 of 250');
  });

  test('rejects when the search API fails', async () => {
    const mockOctokit = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: {
          issuesAndPullRequests: jest.fn().mockRejectedValue(new Error('API rate limit exceeded'))
        }
      }
    };

    await expect(fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'testuser'))
      .rejects.toThrow('API rate limit exceeded');
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
        },
        search: { issuesAndPullRequests: mockQuotaSearch({ mergedCount: 0 }) },
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
        },
        search: { issuesAndPullRequests: mockQuotaSearch({ mergedCount: 0 }) },
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
        },
        search: { issuesAndPullRequests: mockQuotaSearch({ mergedCount: 1 }) },
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
        },
        search: { issuesAndPullRequests: mockQuotaSearch({ mergedCount: 0 }) },
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

  function authorOctokit({ openPRs, mergedCount = 0, closedLabeled = [], searchError = null }) {
    const search = searchError
      ? jest.fn().mockRejectedValue(searchError)
      : mockQuotaSearch({ mergedCount, closedLabeled });
    return {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: openPRs }) },
        search: { issuesAndPullRequests: search },
        issues: {
          getLabel: jest.fn().mockResolvedValue({ data: { name: LABEL_NAME } }),
          addLabels: jest.fn().mockResolvedValue({}),
          removeLabel: jest.fn().mockResolvedValue({}),
          listComments: jest.fn().mockResolvedValue({ data: [] }),
          createComment: jest.fn().mockResolvedValue({})
        }
      }
    };
  }

  test('does not label open PRs when the combined merged count is 3 or more', async () => {
    const openPRs = [1, 2, 3, 4].map(number => ({
      number,
      user: { login: 'contributor' },
      state: 'open',
      created_at: `2024-01-0${number}T00:00:00Z`,
      labels: number === 4 ? [{ name: LABEL_NAME }] : []
    }));
    const mockOctokit = authorOctokit({ openPRs, mergedCount: 5 });

    const result = await processQuotaForAuthor(
      mockOctokit, 'jaegertracing', 'jaeger-ui', 'contributor', mockLogger
    );

    expect(result.mergedCount).toBe(5);
    expect(result.quota).toBe(UNLIMITED_QUOTA);
    expect(result.results.blocked).toEqual([]);
    expect(result.results.unblocked).toEqual([4]);
    expect(mockOctokit.rest.issues.addLabels).not.toHaveBeenCalled();
  });

  test('removes the quota label from closed PRs without commenting', async () => {
    const mockOctokit = authorOctokit({
      openPRs: [],
      mergedCount: 5,
      closedLabeled: [{
        number: 4577,
        repository_url: 'https://api.github.com/repos/jaegertracing/jaeger-ui',
        user: { login: 'contributor' }
      }]
    });

    const result = await processQuotaForAuthor(
      mockOctokit, 'jaegertracing', 'jaeger-ui', 'contributor', mockLogger
    );

    expect(result.results.closedUnlabeled).toEqual([4577]);
    expect(mockOctokit.rest.issues.removeLabel).toHaveBeenCalledWith({
      owner: 'jaegertracing',
      repo: 'jaeger-ui',
      issue_number: 4577,
      name: LABEL_NAME
    });
    expect(mockOctokit.rest.issues.createComment).not.toHaveBeenCalled();
  });

  test('dry run plans closed-label removal without writing', async () => {
    const mockOctokit = authorOctokit({
      openPRs: [],
      mergedCount: 0,
      closedLabeled: [{
        number: 9,
        repository_url: 'https://api.github.com/repos/owner/repo',
        user: { login: 'newuser' }
      }]
    });

    const result = await processQuotaForAuthor(
      mockOctokit, 'owner', 'repo', 'newuser', mockLogger, true
    );

    expect(result.results.closedUnlabeled).toEqual([9]);
    expect(mockOctokit.rest.issues.removeLabel).not.toHaveBeenCalled();
    expect(mockOctokit.rest.issues.getLabel).not.toHaveBeenCalled();
  });

  test('an exempt login is not blocked even with zero merged PRs', async () => {
    const previousExempt = process.env.QUOTA_EXEMPT_USERS;
    process.env.QUOTA_EXEMPT_USERS = 'Other, NewUser';
    mockLogger.log.mockClear();
    try {
      const openPRs = [1, 2, 3].map(number => ({
        number,
        user: { login: 'newuser' },
        created_at: `2024-01-0${number}T00:00:00Z`,
        labels: []
      }));
      const mockOctokit = authorOctokit({ openPRs, mergedCount: 0 });

      const result = await processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'newuser', mockLogger);

      expect(result.quota).toBe(UNLIMITED_QUOTA);
      expect(result.results.blocked).toEqual([]);
      expect(mockOctokit.rest.issues.addLabels).not.toHaveBeenCalled();
      expect(mockOctokit.rest.search.issuesAndPullRequests).not.toHaveBeenCalled();
      const logged = mockLogger.log.mock.calls.map(call => call.join(' ')).join('\n');
      expect(logged).not.toContain('in the project (unlimited quota)');
    } finally {
      if (previousExempt === undefined) delete process.env.QUOTA_EXEMPT_USERS;
      else process.env.QUOTA_EXEMPT_USERS = previousExempt;
    }
  });

  test('dry run does not unlabel an open PR that is back within quota', async () => {
    const openPRs = [{
      number: 1,
      user: { login: 'contributor' },
      created_at: '2024-01-01T00:00:00Z',
      labels: [{ name: LABEL_NAME }]
    }];
    const mockOctokit = authorOctokit({ openPRs, mergedCount: 1 });

    const result = await processQuotaForAuthor(
      mockOctokit, 'owner', 'repo', 'contributor', mockLogger, true
    );

    expect(result.results.unblocked).toEqual([1]);
    expect(mockOctokit.rest.issues.removeLabel).not.toHaveBeenCalled();
    expect(mockOctokit.rest.issues.createComment).not.toHaveBeenCalled();
  });

  test('dry run does not label an open PR that is over quota', async () => {
    const openPRs = [1, 2].map(number => ({
      number,
      user: { login: 'newuser' },
      created_at: `2024-01-0${number}T00:00:00Z`,
      labels: []
    }));
    const mockOctokit = authorOctokit({ openPRs, mergedCount: 0 });

    const result = await processQuotaForAuthor(
      mockOctokit, 'owner', 'repo', 'newuser', mockLogger, true
    );

    expect(result.results.blocked).toEqual([2]);
    expect(mockOctokit.rest.issues.addLabels).not.toHaveBeenCalled();
    expect(mockOctokit.rest.issues.createComment).not.toHaveBeenCalled();
    expect(mockOctokit.rest.issues.getLabel).not.toHaveBeenCalled();
  });

  test('writes nothing when search fails', async () => {
    const mockOctokit = authorOctokit({
      openPRs: [{
        number: 1,
        user: { login: 'newuser' },
        created_at: '2024-01-01T00:00:00Z',
        labels: []
      }],
      searchError: new Error('API rate limit exceeded')
    });

    await expect(
      processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'newuser', mockLogger)
    ).rejects.toThrow('API rate limit exceeded');
    expect(mockOctokit.rest.issues.addLabels).not.toHaveBeenCalled();
    expect(mockOctokit.rest.issues.getLabel).not.toHaveBeenCalled();
    expect(mockOctokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });

  test('writes nothing when the closed-label search fails', async () => {
    const search = jest.fn(async ({ q }) => {
      if (q.includes('is:merged')) {
        return { data: { total_count: 0, incomplete_results: false, items: [] } };
      }
      throw new Error('closed search failed');
    });
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn().mockResolvedValue({
            data: [{
              number: 1,
              user: { login: 'newuser' },
              created_at: '2024-01-01T00:00:00Z',
              labels: []
            }]
          })
        },
        search: { issuesAndPullRequests: search },
        issues: {
          getLabel: jest.fn(),
          addLabels: jest.fn(),
          removeLabel: jest.fn()
        }
      }
    };

    await expect(
      processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'newuser', mockLogger)
    ).rejects.toThrow('closed search failed');
    expect(mockOctokit.rest.issues.addLabels).not.toHaveBeenCalled();
    expect(mockOctokit.rest.issues.getLabel).not.toHaveBeenCalled();
  });

  function closedHit(number, owner, repo, login) {
    return {
      number,
      repository_url: `https://api.github.com/repos/${owner}/${repo}`,
      user: { login }
    };
  }

  test('does not clear a search hit that REST still lists as open', async () => {
    const openPRs = [
      { number: 1, user: { login: 'newuser' }, created_at: '2024-01-01T00:00:00Z', labels: [] },
      { number: 5, user: { login: 'newuser' }, created_at: '2024-01-02T00:00:00Z', labels: [{ name: LABEL_NAME }] }
    ];
    const mockOctokit = authorOctokit({
      openPRs,
      mergedCount: 0,
      closedLabeled: [closedHit(5, 'owner', 'repo', 'newuser')]
    });

    const result = await processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'newuser', mockLogger);

    expect(result.results.unchanged).toContain(5);
    expect(result.results.closedUnlabeled).toEqual([]);
    expect(mockOctokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });

  test('ignores closed search hits for another repository or another login', async () => {
    const mockOctokit = authorOctokit({
      openPRs: [],
      mergedCount: 1,
      closedLabeled: [
        closedHit(3, 'other', 'repo', 'newuser'),
        { number: 4, repository_url: 'https://api.github.com/repos/owner/repo', user: { login: 'someone' } },
        { number: 8, repository: { full_name: 'owner/repo' }, user: { login: 'NewUser' } }
      ]
    });

    const result = await processQuotaForAuthor(mockOctokit, 'owner', 'repo', 'newuser', mockLogger);

    expect(result.results.closedUnlabeled).toEqual([8]);
  });

  test('clears the closed event pull request when search has not indexed it', async () => {
    const mockOctokit = authorOctokit({ openPRs: [], mergedCount: 1, closedLabeled: [] });

    const result = await processQuotaForAuthor(
      mockOctokit, 'owner', 'repo', 'newuser', mockLogger, false,
      { eventName: 'pull_request_target', eventPullRequest: { action: 'closed', number: 4577, merged: false } }
    );

    expect(result.results.closedUnlabeled).toEqual([4577]);
    expect(mockOctokit.rest.issues.removeLabel).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      issue_number: 4577,
      name: LABEL_NAME
    });
  });

  test('counts a just-merged pull request that search has not indexed, crossing 2 to 3', async () => {
    const openPRs = [10, 11, 12, 13].map(number => ({
      number,
      user: { login: 'contributor' },
      created_at: `2024-01-${String(number).padStart(2, '0')}T00:00:00Z`,
      labels: []
    }));
    const search = jest.fn(async ({ q }) => {
      if (q.includes('is:merged')) {
        return {
          data: {
            total_count: 2,
            incomplete_results: false,
            items: [1, 2].map(number => closedHit(number, 'jaegertracing', 'jaeger', 'contributor'))
          }
        };
      }
      return { data: { total_count: 0, incomplete_results: false, items: [] } };
    });
    const mockOctokit = authorOctokit({ openPRs, mergedCount: 0 });
    mockOctokit.rest.search.issuesAndPullRequests = search;

    const result = await processQuotaForAuthor(
      mockOctokit, 'jaegertracing', 'jaeger', 'contributor', mockLogger, false,
      {
        eventName: 'pull_request_target',
        eventPullRequest: { action: 'closed', merged: true, number: 99 }
      }
    );

    expect(result.mergedCount).toBe(3);
    expect(result.quota).toBe(UNLIMITED_QUOTA);
    expect(result.results.blocked).toEqual([]);
    expect(mockOctokit.rest.issues.addLabels).not.toHaveBeenCalled();
  });

  test('does not add one when search already includes the merged pull request', async () => {
    const openPRs = [10, 11, 12].map(number => ({
      number,
      user: { login: 'contributor' },
      created_at: `2024-01-${String(number).padStart(2, '0')}T00:00:00Z`,
      labels: []
    }));
    const search = jest.fn(async ({ q }) => {
      if (q.includes('is:merged')) {
        return {
          data: {
            total_count: 2,
            incomplete_results: false,
            items: [closedHit(1, 'jaegertracing', 'jaeger', 'contributor'), closedHit(99, 'jaegertracing', 'jaeger', 'contributor')]
          }
        };
      }
      return { data: { total_count: 0, incomplete_results: false, items: [] } };
    });
    const mockOctokit = authorOctokit({ openPRs, mergedCount: 0 });
    mockOctokit.rest.search.issuesAndPullRequests = search;

    const result = await processQuotaForAuthor(
      mockOctokit, 'jaegertracing', 'jaeger', 'contributor', mockLogger, false,
      {
        eventName: 'pull_request_target',
        eventPullRequest: { action: 'closed', merged: true, number: 99 }
      }
    );

    expect(result.mergedCount).toBe(2);
    expect(result.quota).toBe(3);
    expect(result.results.blocked).toEqual([]);
  });

  test('queries Copilot as the swe agent app', async () => {
    const search = mockQuotaSearch({ mergedCount: 4 });
    const mockOctokit = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: { issuesAndPullRequests: search }
      }
    };

    await fetchAuthorPRs(mockOctokit, 'jaegertracing', 'jaeger', 'Copilot');

    const queries = search.mock.calls.map(call => call[0].q);
    expect(queries.some(q => q.includes('author:app/copilot-swe-agent'))).toBe(true);
    expect(queries.some(q => q.includes('author:"Copilot"'))).toBe(false);
  });

  test('logs and skips a 422 for an unknown login on a pull request event', async () => {
    const error = Object.assign(new Error('Validation Failed'), {
      status: 422,
      response: { data: { errors: [{ message: 'The listed users cannot be searched' }] } }
    });
    const mockOctokit = authorOctokit({
      openPRs: [{ number: 1, user: { login: 'ghost' }, created_at: '2024-01-01T00:00:00Z', labels: [] }],
      searchError: error
    });

    const result = await processQuotaForAuthor(
      mockOctokit, 'owner', 'repo', 'ghost', mockLogger, false,
      {
        eventName: 'pull_request_target',
        eventPullRequest: { action: 'closed', number: 44, merged: false }
      }
    );

    expect(result.skipped).toBe(true);
    expect(result.results.blocked).toEqual([]);
    expect(result.results.closedUnlabeled).toEqual([44]);
    expect(mockOctokit.rest.issues.addLabels).not.toHaveBeenCalled();
  });

  test('fails a workflow_dispatch run when search returns 422', async () => {
    const error = Object.assign(
      new Error('The listed users cannot be searched either because the users do not exist'),
      { status: 422 }
    );
    const mockOctokit = authorOctokit({
      openPRs: [],
      searchError: error
    });

    await expect(processQuotaForAuthor(
      mockOctokit, 'owner', 'repo', 'not-a-user', mockLogger, false,
      { eventName: 'workflow_dispatch' }
    )).rejects.toThrow('cannot be searched');
    expect(mockOctokit.rest.issues.removeLabel).not.toHaveBeenCalled();
  });

  test('matches open pull requests without regard to login case', async () => {
    const mockOctokit = {
      rest: {
        pulls: {
          list: jest.fn().mockResolvedValue({
            data: [
              { number: 7, user: { login: 'NewUser' } },
              { number: 8, user: null }
            ]
          })
        },
        search: { issuesAndPullRequests: mockQuotaSearch({ mergedCount: 0 }) }
      }
    };

    const result = await fetchAuthorPRs(mockOctokit, 'owner', 'repo', 'newuser');

    expect(result.openPRs.map(pr => pr.number)).toEqual([7]);
  });

  test('does not count a merge from a repository outside the project', async () => {
    const search = jest.fn(async ({ q }) => {
      if (q.includes('is:merged')) {
        return {
          data: {
            total_count: 2,
            incomplete_results: false,
            items: [closedHit(1, 'someone', 'else', 'testuser')]
          }
        };
      }
      return { data: { total_count: 0, incomplete_results: false, items: [] } };
    });
    const mockOctokit = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: { issuesAndPullRequests: search }
      }
    };

    const result = await fetchAuthorPRs(
      mockOctokit, 'someone', 'else', 'testuser',
      { action: 'closed', merged: true, number: 5 }
    );

    expect(result.mergedCount).toBe(2);
  });
});

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

    await prQuotaManager({
      github,
      core,
      owner: 'owner',
      repo: 'repo',
      perIssueLimit: true,
      dryRun: true
    });

    expect(core.info).toHaveBeenCalledWith('Dry-run plan linked issues: none');
    expect(core.info).toHaveBeenCalledWith('Dry-run plan duplicate PRs: none');
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

  test('clears the closed event pull request from the per-issue path', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      graphql: jest.fn().mockResolvedValue({ repository: { pullRequests: { nodes: [] } } }),
      rest: {
        issues: { removeLabel: jest.fn().mockResolvedValue({}) },
        pulls: { list: jest.fn() }
      }
    };

    await prQuotaManager({
      github,
      core,
      owner: 'jaegertracing',
      repo: 'jaeger',
      perIssueLimit: true,
      pullRequestNumber: 15,
      eventName: 'pull_request_target',
      eventPullRequest: { action: 'closed', number: 15, login: 'contributor' }
    });

    expect(github.rest.pulls.list).not.toHaveBeenCalled();
    expect(github.rest.issues.removeLabel).toHaveBeenCalledWith({
      owner: 'jaegertracing',
      repo: 'jaeger',
      issue_number: 15,
      name: LABEL_NAME
    });
    expect(core.info).toHaveBeenCalledWith('Closed event unlabeled: 15');
    expect(core.setFailed).not.toHaveBeenCalled();
  });

  test('dry-runs the closed-label sweep from workflow_dispatch', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [{ number: 4, user: { login: 'a' } }] }) },
        search: {
          issuesAndPullRequests: jest.fn().mockResolvedValue({
            data: {
              total_count: 3,
              incomplete_results: false,
              items: [
                {
                  number: 4,
                  repository_url: 'https://api.github.com/repos/jaegertracing/jaeger',
                  user: { login: 'a' }
                },
                {
                  number: 8,
                  repository_url: 'https://api.github.com/repos/other/repo',
                  user: { login: 'c' }
                },
                {
                  number: 9,
                  repository_url: 'https://api.github.com/repos/jaegertracing/jaeger',
                  user: { login: 'b' }
                }
              ]
            }
          })
        },
        issues: { removeLabel: jest.fn() }
      }
    };

    await prQuotaManager({
      github,
      core,
      owner: 'jaegertracing',
      repo: 'jaeger',
      eventName: 'workflow_dispatch',
      sweepClosedLabels: 'dry-run'
    });

    expect(github.rest.issues.removeLabel).not.toHaveBeenCalled();
    expect(core.info).toHaveBeenCalledWith('Dry-run would clear closed quota labels: 9');
    expect(core.setFailed).not.toHaveBeenCalled();
  });

  test('applies the closed-label sweep only from workflow_dispatch', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: {
          issuesAndPullRequests: jest.fn().mockResolvedValue({
            data: {
              total_count: 1,
              incomplete_results: false,
              items: [{
                number: 9,
                repository_url: 'https://api.github.com/repos/jaegertracing/jaeger',
                user: { login: 'b' }
              }]
            }
          })
        },
        issues: { removeLabel: jest.fn().mockResolvedValue({}) }
      }
    };

    await prQuotaManager({
      github,
      core,
      owner: 'jaegertracing',
      repo: 'jaeger',
      eventName: 'pull_request_target',
      sweepClosedLabels: 'apply'
    });
    expect(core.setFailed).toHaveBeenCalledWith(
      'The closed-label sweep is only available from workflow_dispatch.'
    );
    expect(github.rest.issues.removeLabel).not.toHaveBeenCalled();

    core.setFailed.mockClear();
    await prQuotaManager({
      github,
      core,
      owner: 'jaegertracing',
      repo: 'jaeger',
      eventName: 'workflow_dispatch',
      sweepClosedLabels: 'apply'
    });
    expect(github.rest.issues.removeLabel).toHaveBeenCalledWith({
      owner: 'jaegertracing',
      repo: 'jaeger',
      issue_number: 9,
      name: LABEL_NAME
    });
    expect(core.setFailed).not.toHaveBeenCalled();
  });

  test('forwards a closed pull request into the author quota path', async () => {
    const core = { info: jest.fn(), setFailed: jest.fn() };
    const github = {
      rest: {
        pulls: { list: jest.fn().mockResolvedValue({ data: [] }) },
        search: {
          issuesAndPullRequests: jest.fn().mockResolvedValue({
            data: { total_count: 0, incomplete_results: false, items: [] }
          })
        },
        issues: {
          getLabel: jest.fn().mockResolvedValue({}),
          removeLabel: jest.fn().mockResolvedValue({})
        }
      }
    };

    await prQuotaManager({
      github,
      core,
      username: 'newuser',
      owner: 'owner',
      repo: 'repo',
      eventName: 'pull_request_target',
      eventPullRequest: { action: 'closed', number: 3, merged: false }
    });

    expect(github.rest.issues.removeLabel).toHaveBeenCalledWith({
      owner: 'owner',
      repo: 'repo',
      issue_number: 3,
      name: LABEL_NAME
    });
    expect(core.info).toHaveBeenCalledWith('Closed unlabeled: 1 PRs');
    expect(core.setFailed).not.toHaveBeenCalled();
  });
});
