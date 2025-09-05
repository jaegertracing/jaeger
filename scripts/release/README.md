# Release Automation

This directory contains scripts to automate the Jaeger release process, addressing issue [#7500](https://github.com/jaegertracing/jaeger/issues/7500).

## Overview

The release automation scripts streamline the manual steps in the release process by:

1. **Automating PR creation** with changelog and version updates
2. **Automating tag creation** with user confirmation
3. **Integrating with existing tools** like `make changelog`
4. **Maintaining dual version support** for v1.x.x and v2.x.x

## Setup

### First Time Setup (Unix/Linux/macOS)
```bash
# Make scripts executable
chmod +x scripts/release/*.sh
```

**Important:** This command ensures that the shell scripts can be executed directly without explicitly invoking bash. This is required for the automation to work properly.

### Windows Setup
No additional setup required. The PowerShell script (`automate-release.ps1`) will be used automatically.

## Scripts

### `automate-release.sh` / `automate-release.ps1`

The main automation script, available in both Bash and PowerShell versions.

**Features:**
- Automatic version detection and suggestion
- Changelog generation using existing `make changelog`
- GitHub PR creation with proper labels
- Tag creation automation (optional)
- Cross-platform support (Windows/Linux/macOS)

**Usage:**

> **Note:** On Unix systems, ensure scripts are executable first: `chmod +x scripts/release/*.sh`

```bash
# Interactive mode
make automate-release

# Dry run (no actual changes)
./scripts/release/automate-release.sh --dry-run

# Full automation including tags
./scripts/release/automate-release.sh --auto-tag
```

**PowerShell (Windows):**
```powershell
# Interactive mode
make automate-release

# Dry run
powershell -ExecutionPolicy Bypass -File scripts/release/automate-release.ps1 -DryRun

# Full automation
powershell -ExecutionPolicy Bypass -File scripts/release/automate-release.ps1 -AutoTag
```

### `test-automation.sh`

Test script to verify automation functionality without making changes.

```bash
make test-automation
```

## Prerequisites

1. **GitHub CLI (gh)** - Must be installed and authenticated
2. **Git repository** - Must be in the Jaeger repository root
3. **Main branch** - Should be on main branch (with override option)
4. **Make** - For version detection and changelog generation
5. **Executable permissions** - On Unix systems, ensure scripts are executable:
   ```bash
   chmod +x scripts/release/*.sh
   ```

## Workflow

1. **Run automation script** - Creates PR with changelog updates
2. **Review and merge PR** - Manual review of automated changes
3. **Update UI submodule** - Manual step (if needed)
4. **Create tags** - Automated or manual based on options
5. **Create GitHub release** - Manual step
6. **Trigger CI workflow** - Manual step

## Integration

The automation integrates with existing tools:

- **`make changelog`** - Generates changelog content
- **`make echo-v1`** / **`make echo-v2`** - Gets current versions
- **GitHub CLI** - Creates PRs and manages labels
- **Git** - Creates and pushes tags

## Benefits

- **Reduced manual effort** in release preparation
- **Consistent changelog format** across releases
- **Faster release cycles** with automated PR creation
- **Maintained human oversight** through confirmation steps
- **Cross-platform compatibility** (Windows/Linux/macOS)

## Manual Steps Remaining

As requested in the issue, these steps remain manual:
- Final release creation in GitHub
- Manual triggering of artifact building workflow
- Human review and approval of automated changes

## Testing

Test the automation without making changes:

```bash
# Test basic functionality
make test-automation

# Test in dry-run mode
./scripts/release/automate-release.sh --dry-run
```

## Troubleshooting

### Common Issues

1. **GitHub CLI not authenticated**
   ```bash
   gh auth login
   ```

2. **Not on main branch**
   - Script will warn and ask for confirmation
   - Or switch to main branch first

3. **Make targets not working**
   - Ensure you're in the repository root
   - Check if Make is installed

4. **Permission denied (PowerShell)**
   ```powershell
   Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
   ```

### Error Messages

- **"GitHub CLI not installed"** - Install GitHub CLI
- **"Not authenticated"** - Run `gh auth login`
- **"Not in git repository"** - Run from Jaeger repository root
- **"Failed to create PR"** - Check GitHub CLI permissions and network

## Contributing

When modifying the automation scripts:

1. Test both Bash and PowerShell versions
2. Update this README with any changes
3. Ensure cross-platform compatibility
4. Add appropriate error handling
5. Test in dry-run mode before actual use
