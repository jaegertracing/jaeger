#!/usr/bin/env node

/**
 * List Open PRs Grouped by Author
 * 
 * This utility script lists all open PRs in a repository grouped by author.
 * Useful for identifying which users need quota processing or backfilling.
 * 
 * Usage:
 *   GITHUB_TOKEN=<token> node list-open-prs-by-author.js [owner] [repo]
 */

/**
 * Fetch all open PRs grouped by author
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @returns {Promise<Map>} Map of author -> array of PRs
 */
async function fetchOpenPRsByAuthor(octokit, owner, repo) {
  const prsByAuthor = new Map();
  let page = 1;
  const perPage = 100;

  console.log(`📥 Fetching open PRs from ${owner}/${repo}...`);

  while (true) {
    const { data } = await octokit.rest.pulls.list({
      owner,
      repo,
      state: 'open',
      per_page: perPage,
      page,
      sort: 'created',
      direction: 'asc'
    });

    if (data.length === 0) break;

    for (const pr of data) {
      const author = pr.user.login;
      if (!prsByAuthor.has(author)) {
        prsByAuthor.set(author, []);
      }
      prsByAuthor.get(author).push({
        number: pr.number,
        title: pr.title,
        created_at: pr.created_at,
        labels: pr.labels.map(l => l.name)
      });
    }

    if (data.length < perPage) break;
    page++;
  }

  return prsByAuthor;
}

/**
 * Display PRs grouped by author
 * @param {Map} prsByAuthor - Map of author -> PRs
 */
function displayResults(prsByAuthor) {
  // Sort by number of open PRs (descending)
  const sortedAuthors = Array.from(prsByAuthor.entries())
    .sort((a, b) => b[1].length - a[1].length);

  console.log(`\n📊 Found ${sortedAuthors.length} authors with open PRs\n`);
  console.log('=' .repeat(80));

  for (const [author, prs] of sortedAuthors) {
    const hasQuotaLabel = prs.some(pr => pr.labels.includes('pr-quota-reached'));
    const quotaIndicator = hasQuotaLabel ? ' 🚫' : '';
    
    console.log(`\n👤 @${author} (${prs.length} open PR${prs.length > 1 ? 's' : ''})${quotaIndicator}`);
    
    // Sort PRs by creation date (oldest first)
    const sortedPRs = prs.sort((a, b) => 
      new Date(a.created_at) - new Date(b.created_at)
    );
    
    for (const pr of sortedPRs) {
      const date = new Date(pr.created_at).toISOString().split('T')[0];
      const quotaLabel = pr.labels.includes('pr-quota-reached') ? ' [QUOTA REACHED]' : '';
      console.log(`   - PR #${pr.number}: ${pr.title.substring(0, 70)}${pr.title.length > 70 ? '...' : ''}`);
      console.log(`     Created: ${date}${quotaLabel}`);
    }
  }

  console.log('\n' + '='.repeat(80));
  console.log(`\n📋 Summary:`);
  console.log(`   Total authors: ${sortedAuthors.length}`);
  console.log(`   Total open PRs: ${Array.from(prsByAuthor.values()).reduce((sum, prs) => sum + prs.length, 0)}`);
  
  const authorsWithQuota = sortedAuthors.filter(([_, prs]) => 
    prs.some(pr => pr.labels.includes('pr-quota-reached'))
  ).length;
  if (authorsWithQuota > 0) {
    console.log(`   Authors with quota-blocked PRs: ${authorsWithQuota}`);
  }
}

/**
 * Display in CSV format for easy processing
 * @param {Map} prsByAuthor - Map of author -> PRs
 */
function displayCSV(prsByAuthor) {
  console.log('Author,PR Count,PR Numbers,Has Quota Label');
  
  for (const [author, prs] of prsByAuthor.entries()) {
    const prNumbers = prs.map(pr => `#${pr.number}`).join(' ');
    const hasQuotaLabel = prs.some(pr => pr.labels.includes('pr-quota-reached'));
    console.log(`${author},${prs.length},"${prNumbers}",${hasQuotaLabel}`);
  }
}

/**
 * Main execution function
 */
async function main() {
  const args = process.argv.slice(2);
  
  const owner = args[0] || process.env.GITHUB_REPOSITORY?.split('/')[0] || 'jaegertracing';
  const repo = args[1] || process.env.GITHUB_REPOSITORY?.split('/')[1] || 'jaeger';
  const format = process.env.FORMAT || 'default'; // 'default' or 'csv'

  if (!process.env.GITHUB_TOKEN) {
    console.error('Error: GITHUB_TOKEN environment variable is required');
    console.error('Usage: GITHUB_TOKEN=<token> node list-open-prs-by-author.js [owner] [repo]');
    console.error('Optional: FORMAT=csv for CSV output');
    process.exit(1);
  }

  // Import @octokit/rest dynamically
  const { Octokit } = await import('@octokit/rest');
  const octokit = new Octokit({
    auth: process.env.GITHUB_TOKEN
  });

  try {
    const prsByAuthor = await fetchOpenPRsByAuthor(octokit, owner, repo);
    
    if (format === 'csv') {
      displayCSV(prsByAuthor);
    } else {
      displayResults(prsByAuthor);
    }
  } catch (error) {
    console.error('Error:', error.message);
    process.exit(1);
  }
}

// Export for testing
if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    fetchOpenPRsByAuthor,
    displayResults,
    displayCSV
  };
}

// Run main function if executed directly
if (require.main === module) {
  main().catch(error => {
    console.error('Fatal error:', error);
    process.exit(1);
  });
}
