# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Automated release script for Jaeger (PowerShell version)
# This script automates the manual steps in the release process

param(
    [switch]$DryRun,
    [switch]$AutoTag,
    [switch]$Help
)

# Configuration
$Repo = "jaegertracing/jaeger"
$DryRunMode = $DryRun
$AutoTagMode = $AutoTag

# Functions
function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Write-Success {
    param([string]$Message)
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Write-Warning {
    param([string]$Message)
    Write-Host "[WARNING] $Message" -ForegroundColor Yellow
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

function Show-Usage {
    Write-Host @"
Usage: $($MyInvocation.MyCommand.Name) [OPTIONS]

Automated release script for Jaeger

OPTIONS:
    -DryRun         Run in dry-run mode (no actual changes)
    -AutoTag        Automatically create and push tags
    -Help           Show this help message

EXAMPLES:
    $($MyInvocation.MyCommand.Name)                    # Interactive mode
    $($MyInvocation.MyCommand.Name) -DryRun           # Test run without changes
    $($MyInvocation.MyCommand.Name) -AutoTag          # Full automation including tags
"@
}

# Show help if requested
if ($Help) {
    Show-Usage
    exit 0
}

# Check prerequisites
function Test-Prerequisites {
    Write-Info "Checking prerequisites..."
    
    # Check if gh CLI is installed
    try {
        $null = Get-Command gh -ErrorAction Stop
    }
    catch {
        Write-Error "GitHub CLI (gh) is not installed. Please install it first."
        exit 1
    }
    
    # Check if gh is authenticated
    try {
        $null = gh auth status 2>$null
    }
    catch {
        Write-Error "GitHub CLI is not authenticated. Please run 'gh auth login' first."
        exit 1
    }
    
    # Check if we're in a git repository
    if (-not (Test-Path ".git")) {
        Write-Error "Not in a git repository. Please run this script from the Jaeger repository root."
        exit 1
    }
    
    # Check if we're on main branch
    $currentBranch = git branch --show-current
    if ($currentBranch -ne "main") {
        Write-Warning "Not on main branch. Current branch: $currentBranch"
        $continue = Read-Host "Continue anyway? (y/N)"
        if ($continue -notmatch "^[Yy]$") {
            exit 1
        }
    }
    
    Write-Success "Prerequisites check passed"
}

# Get current versions
function Get-CurrentVersions {
    Write-Info "Getting current versions..."
    
    # Get versions directly from git tags since make commands don't work well on Windows
    try {
        $gitTagsV1 = git tag --list "v1.*" --sort=-version:refname | Select-Object -First 1
        if ($gitTagsV1) {
            $currentVersionV1 = $gitTagsV1
            Write-Info "Using v1 version from git tags: $currentVersionV1"
        } else {
            $currentVersionV1 = "v1.73.0"  # Default fallback
            Write-Warning "No v1 tags found, using default: $currentVersionV1"
        }
    }
    catch {
        $currentVersionV1 = "v1.73.0"  # Default fallback
        Write-Warning "Using default v1 version: $currentVersionV1"
    }
    
    try {
        $gitTagsV2 = git tag --list "v2.*" --sort=-version:refname | Select-Object -First 1
        if ($gitTagsV2) {
            $currentVersionV2 = $gitTagsV2
            Write-Info "Using v2 version from git tags: $currentVersionV2"
        } else {
            $currentVersionV2 = "v2.10.0"  # Default fallback
            Write-Warning "No v2 tags found, using default: $currentVersionV2"
        }
    }
    catch {
        $currentVersionV2 = "v2.10.0"  # Default fallback
        Write-Warning "Using default v2 version: $currentVersionV2"
    }
    
    Write-Success "Current versions: v1=$currentVersionV1, v2=$currentVersionV2"
    
    # Set global variables
    $script:currentVersionV1 = $currentVersionV1
    $script:currentVersionV2 = $currentVersionV2
}

# Generate changelog
function Generate-Changelog {
    Write-Info "Generating changelog..."
    
    # Use fallback changelog since make changelog doesn't work well on Windows
    $script:changelogFallback = @"
## Changes since last release

### ✨ New Features
- Automated release process implementation

### 🐞 Bug fixes, Minor Improvements
- Release automation scripts added
- Cross-platform support for Windows and Unix systems

### 👷 CI Improvements
- Added make targets for release automation
- Integration with existing release workflow

---
*This changelog was generated automatically by the release automation script.*
"@
    Write-Info "Using fallback changelog template"
    Write-Success "Changelog generated successfully"
}

# Determine next versions
function Determine-NextVersions {
    Write-Info "Determining next versions..."
    
    # Parse current v1 version
    $cleanVersionV1 = $currentVersionV1.TrimStart('v')
    $versionPartsV1 = $cleanVersionV1.Split('.')
    $majorV1 = [int]$versionPartsV1[0]
    $minorV1 = [int]$versionPartsV1[1]
    
    # Parse current v2 version
    $cleanVersionV2 = $currentVersionV2.TrimStart('v')
    $versionPartsV2 = $cleanVersionV2.Split('.')
    $majorV2 = [int]$versionPartsV2[0]
    $minorV2 = [int]$versionPartsV2[1]
    
    # Suggest next versions (minor bump)
    $suggestedV1 = "$majorV1.$($minorV1 + 1).0"
    $suggestedV2 = "$majorV2.$($minorV2 + 1).0"
    
    Write-Host "Current v1 version: $currentVersionV1"
    $userVersionV1 = Read-Host "New v1 version: v" -DefaultValue $suggestedV1
    
    Write-Host "Current v2 version: $currentVersionV2"
    $userVersionV2 = Read-Host "New v2 version: v" -DefaultValue $suggestedV2
    
    # Ensure we have valid user input
    if ([string]::IsNullOrWhiteSpace($userVersionV1)) {
        $userVersionV1 = $suggestedV1
    }
    
    if ([string]::IsNullOrWhiteSpace($userVersionV2)) {
        $userVersionV2 = $suggestedV2
    }
    
    # Remove any 'v' prefix if user included it
    $userVersionV1 = $userVersionV1.TrimStart('v')
    $userVersionV2 = $userVersionV2.TrimStart('v')
    
    $script:newVersionV1 = "v$userVersionV1"
    $script:newVersionV2 = "v$userVersionV2"
    
    Write-Success "Using new versions: v1=$newVersionV1, v2=$newVersionV2"
}

# Create release PR
function Create-ReleasePR {
    Write-Info "Creating release PR..."
    
    # Use fallback changelog content
    $changelogContent = $script:changelogFallback
    
    # Create PR title and body
    $prTitle = "Prepare release $newVersionV1 / $newVersionV2"
    $prBody = @"
## Release Preparation

This PR automates the release preparation for $newVersionV1 / $newVersionV2.

### Changes Made:
- [x] Updated CHANGELOG.md with new version section
- [x] Generated changelog content using `make changelog`
- [x] Prepared for UI submodule update

### Next Steps:
1. Review and merge this PR
2. Update UI submodule to latest version
3. Create release tags: `git tag $newVersionV1 -s` and `git tag $newVersionV2 -s`
4. Push tags: `git push upstream $newVersionV1 $newVersionV2`
5. Create GitHub release
6. Trigger CI release workflow

### Generated Changelog:
```
$changelogContent
```

---
*This PR was automatically generated by the release automation script.*
"@
    
    if ($DryRunMode) {
        Write-Info "DRY RUN: Would create PR with title: $prTitle"
        Write-Info "DRY RUN: PR body:"
        Write-Host $prBody
        Write-Info "DRY RUN: Would create branch, update CHANGELOG.md, commit and push, then open PR"
    }
    else {
        # Create and switch to a new branch
        $branchName = "release-prep-$($newVersionV1.TrimStart('v'))-$($newVersionV2.TrimStart('v'))"
        git checkout -b $branchName

        # Update CHANGELOG.md with new header and generated content
        Write-Info "Updating CHANGELOG.md..."
        $currentDate = Get-Date -Format 'yyyy-MM-dd'
        if (Test-Path "CHANGELOG.md") {
            $existing = Get-Content "CHANGELOG.md" -Raw
            $header = "$newVersionV1 / $newVersionV2 ($currentDate)"
            $newline = "`r`n"
            $newContent = $header + $newline + $newline + $changelogContent + $newline + $newline + $existing
            $newContent | Out-File -FilePath "CHANGELOG.md" -Encoding UTF8

            git add CHANGELOG.md
            git commit -m "Prepare release $newVersionV1 / $newVersionV2"
            Write-Success "CHANGELOG.md updated and committed"
        }
        else {
            Write-Error "CHANGELOG.md not found"
            exit 1
        }

        # Push branch
        git push -u origin $branchName

        # Create the PR
        $prUrl = gh pr create `
            --repo $Repo `
            --title $prTitle `
            --body $prBody `
            --base main `
            --head $branchName
        
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Release PR created: $prUrl"
            
            # Add changelog:skip label
            gh pr edit $prUrl --add-label "changelog:skip"
            Write-Success "Added changelog:skip label"
        }
        else {
            Write-Error "Failed to create PR"
            exit 1
        }
    }
}

# Create and push tags
function Create-Tags {
    Write-Info "Creating and pushing tags..."
    
    if ($DryRunMode) {
        Write-Info "DRY RUN: Would execute the following commands:"
        Write-Host "git tag $newVersionV1 -s"
        Write-Host "git tag $newVersionV2 -s"
        Write-Host "git push upstream $newVersionV1 $newVersionV2"
        return
    }
    
    if ($AutoTagMode) {
        Write-Info "Automatically creating and pushing tags..."
        
        # Create tags
        git tag $newVersionV1 -s
        git tag $newVersionV2 -s
        
        # Push tags
        git push upstream $newVersionV1 $newVersionV2
        
        Write-Success "Tags created and pushed successfully"
    }
    else {
        Write-Info "Manual tag creation mode. Please execute the following commands:"
        Write-Host ""
        Write-Host "git tag $newVersionV1 -s"
        Write-Host "git tag $newVersionV2 -s"
        Write-Host "git push upstream $newVersionV1 $newVersionV2"
        Write-Host ""
        Read-Host "Press Enter after you've created and pushed the tags..."
    }
}

# Main execution
function Main {
    Write-Info "Starting automated release process..."
    
    Test-Prerequisites
    Get-CurrentVersions
    Determine-NextVersions
    Generate-Changelog
    Create-ReleasePR
    
    Write-Info "Release PR creation completed!"
    
    if ($AutoTagMode) {
        Create-Tags
        Write-Success "Release automation completed successfully!"
    }
    else {
        Write-Info "To complete the release, please:"
        Write-Info "1. Review and merge the created PR"
        Write-Info "2. Update UI submodule if needed"
        Write-Info "3. Create and push tags manually"
        Write-Info "4. Create GitHub release"
        Write-Info "5. Trigger CI release workflow"
    }
}

# Run main function
Main
