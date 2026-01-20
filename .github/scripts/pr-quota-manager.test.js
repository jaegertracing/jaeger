/**
 * Unit tests for PR Quota Management System
 */

const prQuotaManager = require('./pr-quota-manager');
const {
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

  test('handles errors gracefully', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          addLabels: jest.fn().mockRejectedValue(new Error('API error'))
        }
      }
    };

    await addLabel(mockOctokit, 'owner', 'repo', 123, mockLogger);

    expect(mockLogger.error).toHaveBeenCalledWith(
      expect.stringContaining('Failed to add label'),
      expect.any(String)
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

    await removeLabel(mockOctokit, 'owner', 'repo', 123, testLogger);

    expect(testLogger.error).not.toHaveBeenCalled();
  });

  test('logs non-404 errors', async () => {
    const mockOctokit = {
      rest: {
        issues: {
          removeLabel: jest.fn().mockRejectedValue({ status: 500, message: 'Server error' })
        }
      }
    };

    await removeLabel(mockOctokit, 'owner', 'repo', 123, mockLogger);

    expect(mockLogger.error).toHaveBeenCalled();
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
