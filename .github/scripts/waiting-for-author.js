module.exports = async ({github, context, core}) => {
  const LABEL_NAME = 'waiting-for-author';

  // Determine event type
  const eventName = context.eventName;
  
  // Get PR data
  let prNumber;
  let repoOwner;
  let repoName;

  if (eventName === 'issue_comment') {
    prNumber = context.payload.issue.number;
    repoOwner = context.repo.owner;
    repoName = context.repo.repo;
  } else if (eventName === 'pull_request_target') {
    prNumber = context.payload.number;
    repoOwner = context.repo.owner;
    repoName = context.repo.repo;
  } else {
    core.info(`Unsupported event: ${eventName}`);
    return;
  }

  // Fetch PR details to get the author
  // We need to fetch the PR object because issue_comment payload doesn't always have full PR details (like author)
  // correctly populated in a way that is identical to pull_request payload for our needs.
  const { data: pr } = await github.rest.pulls.get({
    owner: repoOwner,
    repo: repoName,
    pull_number: prNumber,
  });

  const prAuthor = pr.user.login;

  if (eventName === 'issue_comment') {
    const commenter = context.payload.comment.user.login;
    
    // Logic:
    // If Maintainer comments -> Add label (if not present)
    // If Author comments -> Remove label (if present)
    
    // Check if commenter is the author
    if (commenter === prAuthor) {
      core.info(`Comment by author ${commenter}. Removing label if present.`);
      await removeLabel(github, repoOwner, repoName, prNumber, LABEL_NAME);
    } 
    // Check if commenter is a maintainer (has write access)
    else if (await isMaintainer(github, repoOwner, repoName, commenter)) {
       core.info(`Comment by maintainer ${commenter}. Adding label if missing.`);
       await addLabel(github, repoOwner, repoName, prNumber, LABEL_NAME);
    } else {
      core.info(`Comment by ${commenter} (not author or maintainer). No action taken.`);
    }

  } else if (eventName === 'pull_request_target') {
    // This is the 'synchronize' event (push to PR branch)
    // Logic:
    // If Author pushes -> Remove label (UNLESS it's just an "Update branch" merge)
    
    const sender = context.payload.sender.login;
    if (sender !== prAuthor) {
       core.info(`Push by ${sender}, not the PR author ${prAuthor}. Doing nothing.`);
       return;
    }

    // Check if it's an "Update branch" commit
    // We look at the commits in this push.
    // context.payload.before and context.payload.after give us the range of commits.
    // simpler approach: look at the head commit of the PR.
    
    // Ideally we want to see if the content changed or if it was just a merge from base.
    // A heuristic is to check the commit message or parents of the head commit.
    
    // We'll fetch the commit details.
    const headSha = context.payload.pull_request.head.sha;
    const { data: commit } = await github.rest.repos.getCommit({
      owner: repoOwner,
      repo: repoName,
      ref: headSha,
    });

    const message = commit.commit.message;
    const parents = commit.parents;

    // A merge commit typically has 2 parents.
    // If it's a merge from the base branch (e.g. "Merge branch 'main' into ...")
    // Note: GitHub's "Update branch" button creates a merge commit.
    
    const isMergeCommit = parents.length > 1;
    const isUpdateBranch = isMergeCommit && (
        message.startsWith(`Merge branch '${context.payload.pull_request.base.ref}'`) || 
        message.startsWith(`Merge remote-tracking branch 'origin/${context.payload.pull_request.base.ref}'`)
    );

    if (isUpdateBranch) {
      core.info(`Push detected as 'Update branch' (Merge from base). Keeping label.`);
      return;
    }

    core.info(`Push by author detected. Removing label.`);
    await removeLabel(github, repoOwner, repoName, prNumber, LABEL_NAME);
  }

};

async function addLabel(github, owner, repo, issueNumber, label) {
  try {
    const { data: labels } = await github.rest.issues.listLabelsOnIssue({
      owner,
      repo,
      issue_number: issueNumber,
    });
    
    if (labels.find(l => l.name === label)) {
      console.log(`Label '${label}' already exists.`);
      return;
    }

    await github.rest.issues.addLabels({
      owner,
      repo,
      issue_number: issueNumber,
      labels: [label],
    });
    console.log(`Added label '${label}'.`);
  } catch (error) {
    console.error(`Error adding label: ${error.message}`);
  }
}

async function removeLabel(github, owner, repo, issueNumber, label) {
  try {
    const { data: labels } = await github.rest.issues.listLabelsOnIssue({
      owner,
      repo,
      issue_number: issueNumber,
    });
    
    if (!labels.find(l => l.name === label)) {
      console.log(`Label '${label}' does not exist.`);
      return;
    }

    await github.rest.issues.removeLabel({
      owner,
      repo,
      issue_number: issueNumber,
      name: label,
    });
    console.log(`Removed label '${label}'.`);
  } catch (error) {
    // Ignore 404 if label not found (though check above should catch it)
    console.error(`Error removing label: ${error.message}`);
  }
}

async function isMaintainer(github, owner, repo, username) {
  try {
    const { data } = await github.rest.repos.getCollaboratorPermissionLevel({
      owner,
      repo,
      username,
    });
    
    // Based on gh api logic: .permissions.maintain==true or .permissions.admin==true or .permissions.push==true
    // getCollaboratorPermissionLevel returns a 'permission' field which describes the permission level.
    // Levels: 'admin', 'maintain', 'write', 'triage', 'read', 'none'
    // We want 'admin', 'maintain', or 'write'.
    const permission = data.permission;
    return ['admin', 'maintain', 'write'].includes(permission);
  } catch (error) {
    console.error(`Error checking permissions for ${username}: ${error.message}`);
    return false;
  }
}
