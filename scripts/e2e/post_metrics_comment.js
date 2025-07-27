const fs = require('fs');
const core = require('@actions/core');
const github = require('@actions/github');

async function run() {
    try {
        const token = core.getInput('github-token');
        const summaryPath = core.getInput('summary-path');
        const runId = core.getInput('run-id');

        const octokit = github.getOctokit(token);
        const { owner, repo } = github.context.repo;
        const { number: issue_number } = github.context.issue;

        const summary = fs.readFileSync(summaryPath, 'utf8');
        const artifactUrl = `https://github.com/${owner}/${repo}/actions/runs/${runId}/artifacts`;
        const commentBody = summary.replace('$LINK_TO_ARTIFACT', artifactUrl) + '\n<!-- METRICS_COMMENT -->';

        // Find existing comment
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
        } else {
            await octokit.rest.issues.createComment({
                owner,
                repo,
                issue_number,
                body: commentBody
            });
        }
    } catch (err) {
        core.error(`Failed to post PR comment: ${err.message}`);
        process.exit(1);
    }
}

run();