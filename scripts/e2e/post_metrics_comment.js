const fs = require('fs');
const core = require('@actions/core');
const github = require('@actions/github');

async function run() {
    try {
        // Get all inputs from environment variables
        const token = process.env.GITHUB_TOKEN;
        const summaryPath = process.env.SUMMARY_PATH;
        const runId = process.env.RUN_ID;

        // Validate required inputs
        if (!token) throw new Error('Missing GITHUB_TOKEN');
        if (!summaryPath) throw new Error('Missing SUMMARY_PATH');
        if (!runId) throw new Error('Missing RUN_ID');

        const octokit = github.getOctokit(token);
        const { owner, repo } = github.context.repo;
        const { number: issue_number } = github.context.issue;

        // Read and prepare comment content
        const summary = fs.readFileSync(summaryPath, 'utf8');
        const artifactUrl = `https://github.com/${owner}/${repo}/actions/runs/${runId}/artifacts`;
        const commentBody = summary.replace('$LINK_TO_ARTIFACT', artifactUrl) + '\n<!-- METRICS_COMMENT -->';

        // Find and update or create comment
        const { data: comments } = await octokit.rest.issues.listComments({
            owner,
            repo,
            issue_number
        });

        const metricsComment = comments.find(c => c.body.includes('<!-- METRICS_COMMENT -->'));

        if (metricsComment) {
            await octokit.rest.issues.updateComment({
                owner,
                repo,
                comment_id: metricsComment.id,
                body: commentBody
            });
            core.info('Updated existing metrics comment');
        } else {
            await octokit.rest.issues.createComment({
                owner,
                repo,
                issue_number,
                body: commentBody
            });
            core.info('Created new metrics comment');
        }
    } catch (err) {
        core.error(`Failed to post PR comment: ${err.message}`);
        process.exit(1);
    }
}

run();