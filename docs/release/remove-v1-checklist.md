# Remove v1 Release Logic - Migration Checklist

**Related Issue:** [#7497](https://github.com/jaegertracing/jaeger/issues/7497)  
**Owner/Reviewer:** @yurishkuro  
**Created:** 2025-11-12

## Purpose and Scope

This checklist provides a comprehensive plan to remove all v1 release logic from the Jaeger repository and transition to v2-only releases. This is a **clean-cut removal** with no feature flags or backwards compatibility maintained in the release infrastructure.

### Goals

1. **Simplify Release Process**: Eliminate dual v1/v2 release paths to reduce complexity and maintenance burden
2. **Reduce Technical Debt**: Remove legacy v1 release infrastructure that is no longer needed
3. **Streamline CI/CD**: Simplify build and deployment pipelines by removing v1-specific logic
4. **Update Documentation**: Ensure all docs reflect v2-first approach
5. **Modernize Defaults**: Update all examples and docker-compose files to use v2 images by default

### Repository Analysis

A comprehensive repository scan identified **475+ occurrences** of 'v1' across scripts, makefiles, CI workflows, and documentation. This checklist covers all files that MUST be updated to complete the migration. Not every line mentioning v1 needs changes (e.g., historical changelog entries), but all files listed below require review and modification.

---

## Rollout Plan

### Phase 1: Update Scripts and Build Infrastructure (Weeks 1-2)
- Update all build scripts to use v2 version computation only
- Modify Makefiles to remove v1 targets and variables
- Update CI workflows to publish v2 artifacts only
- Test release process in staging/dry-run mode

### Phase 2: Production Releases with v2-Only (Weeks 3-4)
- Perform first production release using v2-only infrastructure
- Monitor release process and fix any issues
- Update documentation to reflect new process

### Phase 3: Cleanup and Final Removal (2026)
- Remove v1 Docker images from registry after deprecation period
- Archive old v1 release artifacts
- Final cleanup of any remaining v1 references

---

## Prioritized File Checklist

### Critical Priority (Must Change First)

These files directly control the release process and version computation. Changes here are required before any release can be made with v2-only logic.

#### Build and Version Management

- [ ] **scripts/makefiles/BuildInfo.mk**
  - Remove `GIT_CLOSEST_TAG_V1` variable definition
  - Remove `BUILD_INFO` variable (keep `BUILD_INFO_V2` only)
  - Update to use only v2 version computation
  - Command: Remove lines computing `GIT_CLOSEST_TAG_V1` and change `BUILD_INFO` references to `BUILD_INFO_V2`

- [ ] **scripts/utils/compute-version.sh**
  - Remove v1 version computation logic
  - Make script default to v2 or remove version parameter
  - Ensure script only returns v2 semver tags
  - Command: `# Remove v1 branch/case from version computation logic`

- [ ] **scripts/utils/compute-tags.sh**
  - Update to compute only v2 tags
  - Remove any v1 tag filtering or computation
  - Command: `# Filter for v2.* tags only, remove v1.* logic`

#### Core Build Makefiles

- [ ] **Makefile**
  - Remove `echo-v1` target (line ~97-99)
  - Update any references to `GIT_CLOSEST_TAG_V1` to use `GIT_CLOSEST_TAG_V2`
  - Verify no other v1-specific targets exist
  - Command: Remove target definition and update variable references

- [ ] **scripts/makefiles/BuildBinaries.mk**
  - Update binary build targets to use `BUILD_INFO_V2` only
  - Remove any v1-specific build flags or targets
  - Ensure all binaries are built with v2 version information
  - Command: `# Replace BUILD_INFO with BUILD_INFO_V2 in go build commands`

#### Release Scripts

- [ ] **scripts/release/start.sh**
  - Update to prompt for v2 version only
  - Remove v1 version input and validation
  - Update generated release checklist template to be v2-only
  - Command: `# Remove v1.x.x version prompts, keep only v2.x.x`

- [ ] **scripts/release/formatter.py**
  - Update version formatting logic to handle v2 only
  - Remove v1 version string parsing/formatting
  - Command: `# Remove v1 version format patterns from regex/parsing`

- [ ] **scripts/release/draft.py**
  - Update draft release creation to use v2 version
  - Remove v1 tag references from draft content
  - Command: `# Update tag parsing to only look for v2.* tags`

- [ ] **scripts/release/notes.py**
  - Update release notes generation for v2 only
  - Remove v1 version references from note templates
  - Command: `# Filter release notes to v2 versions only`

### High Priority (CI/CD and Deployment)

These files control automated builds and deployments. Must be updated before running automated releases.

#### GitHub Actions Workflows

- [ ] **.github/workflows/ci-release.yml**
  - Remove v1 tag publish steps
  - Update to publish only v2 Docker images
  - Remove v1 artifact creation and upload
  - Update release job to tag and push v2 only
  - Command: `# Remove steps with v1 tags/versions, keep v2 steps only`

- [ ] **.github/workflows/ci-docker-build.yml**
  - Update Docker build to use v2 version tags
  - Remove v1 image tag generation
  - Ensure only v2 images are built for PRs/branches
  - Command: `# Update docker tag logic to use VERSION_V2, remove VERSION_V1`

- [ ] **.github/workflows/ci-docker-hotrod.yml**
  - Update hotrod example image builds to use v2 versioning
  - Remove v1 tag references
  - Command: `# Use v2 version for hotrod image tags`

#### Package and Deploy Scripts

- [ ] **scripts/build/package-deploy.sh**
  - Update to package only v2 binaries
  - Remove v1 versioning from package names
  - Update artifact paths to use VERSION_V2
  - Command: `# Change VERSION_V1 to VERSION_V2 in package names and paths`

- [ ] **scripts/build/build-upload-a-docker-image.sh**
  - Update to build and tag v2 images only
  - Remove v1 tag logic
  - Ensure only v2 semantic version tags are applied
  - Command: `# Remove v1 tag references, use v2 version for all tags`

### Medium Priority (Documentation and Examples)

These files affect user-facing documentation and examples. Should be updated before public announcement.

#### Release Documentation

- [ ] **RELEASE.md**
  - Update release process to describe v2-only workflow
  - Remove references to "v1.x.x / v2.x.x" dual versioning (line ~5, 17-18, etc.)
  - Update instructions to use v2 version tags only
  - Update commands to push only `v2.x.x` tags (line ~40-42)
  - Command: `sed -i 's/v1\.x\.x \/ v2\.x\.x/v2.x.x/g' RELEASE.md` and manually review

- [ ] **CHANGELOG.md**
  - Update template at top to use v2 version only (line ~11)
  - Keep historical v1 entries intact for reference
  - Future releases should use v2.x.x format only
  - Command: `# Update next release template to "next release v2.x.x (yyyy-mm-dd)"`

- [ ] **CONTRIBUTING.md**
  - Review and update any release process references
  - Ensure contributor docs reflect v2-only approach
  - Update version examples to use v2.x.x format
  - Command: `# Search and update version examples to v2.x.x`

#### Examples and Docker Compose

- [ ] **docker-compose/monitor/Makefile**
  - Update to pull v2 Jaeger images by default
  - Remove v1 version references
  - Command: `# Update JAEGER_VERSION to default to v2 tag or latest v2`

- [ ] **docker-compose/tail-sampling/Makefile**
  - Update to use v2 Jaeger images
  - Remove v1 image tag references
  - Command: `# Update JAEGER_VERSION to use v2 tags`

- [ ] **examples/otel-demo/deploy-all.sh**
  - Update deployment script to use v2 Jaeger images
  - Remove v1 version logic
  - Command: `# Update image tags to use v2 versions`

### Low Priority (Auxiliary and Testing)

These files are less critical but should be updated for consistency and to avoid confusion.

#### Testing and Scripts

- [ ] **scripts/e2e/elasticsearch.sh**
  - Update e2e tests to default to v2 binaries
  - Remove v1 test targets
  - Command: `# Update test script to use v2 binary paths/versions`

- [ ] **scripts/utils/compare_metrics.py**
  - Update version parsing if used for metrics comparison
  - Ensure only v2 metrics are compared
  - Command: `# Update version regex to match v2.x.x only`

- [ ] **scripts/lint/check-go-version.sh**
  - Review for any v1 version checks
  - Update if script validates version formats
  - Command: `# Verify no v1 version format checks remain`

#### Additional Files to Review

- [ ] **scripts/release/*** (scan all files in directory)
  - Review all other scripts in release directory
  - Update any remaining v1 references
  - Command: `grep -r "v1" scripts/release/ | grep -v ".git" | grep -v "binary"`

- [ ] **docs/release/remove-v1-checklist.md** (this file)
  - Mark all items complete when done
  - Archive or move to completed-migrations folder after completion

---

## Detailed Change Instructions by File

### 1. scripts/makefiles/BuildInfo.mk

**Current state:** Defines both `GIT_CLOSEST_TAG_V1` and `GIT_CLOSEST_TAG_V2`, plus `BUILD_INFO` and `BUILD_INFO_V2`

**Changes required:**
```bash
# Remove these lines:
GIT_CLOSEST_TAG_V1 = $(eval GIT_CLOSEST_TAG_V1 := $(shell scripts/utils/compute-version.sh v1))$(GIT_CLOSEST_TAG_V1)
BUILD_INFO=$(call buildinfoflags,V1)

# Keep only:
GIT_CLOSEST_TAG_V2 = $(eval GIT_CLOSEST_TAG_V2 := $(shell scripts/utils/compute-version.sh v2))$(GIT_CLOSEST_TAG_V2)
BUILD_INFO_V2=$(call buildinfoflags,V2)

# Or rename BUILD_INFO_V2 back to BUILD_INFO if preferred
```

### 2. Makefile

**Current state:** Contains `echo-v1` target at line 97-99

**Changes required:**
```bash
# Remove target:
.PHONY: echo-v1
echo-v1:
	@echo "$(GIT_CLOSEST_TAG_V1)"

# Optional: Add echo-version that uses v2 only if needed
.PHONY: echo-version
echo-version:
	@echo "$(GIT_CLOSEST_TAG_V2)"
```

### 3. scripts/utils/compute-version.sh

**Changes required:**
- Remove v1 branch in version computation logic
- Make script accept only v2 or remove parameter entirely
- Update git describe commands to filter v2.* tags only

```bash
# Update tag filtering:
git describe --tags --match="v2.*" --abbrev=0
# Remove any --match="v1.*" logic
```

### 4. CI Workflows (.github/workflows/*.yml)

**Changes required:**
- Remove steps that build/push v1 tags
- Update Docker tag logic to use only v2 versions
- Remove v1 artifact uploads
- Update matrix builds to use VERSION_V2 only

```yaml
# Example change in ci-release.yml:
# Remove:
- name: Publish v1 Docker images
  run: |
    make docker-push VERSION=${VERSION_V1}

# Keep only:
- name: Publish v2 Docker images
  run: |
    make docker-push VERSION=${VERSION_V2}
```

### 5. scripts/release/*.py and *.sh

**Changes required:**
- Update version input prompts to request v2 version only
- Remove v1 version parsing and validation
- Update release note templates to single version format
- Update tag filtering to v2.* only

```python
# Example in formatter.py:
# Change version regex from:
VERSION_PATTERN = r'v[12]\.\d+\.\d+'
# To:
VERSION_PATTERN = r'v2\.\d+\.\d+'
```

### 6. RELEASE.md

**Changes required:**
- Update all references from "v1.x.x / v2.x.x" to just "v2.x.x"
- Update tag commands to push single v2 tag
- Simplify release instructions

```bash
# Old:
git tag v1.x.x -s
git tag v2.x.x -s
git push upstream v1.x.x v2.x.x

# New:
git tag v2.x.x -s
git push upstream v2.x.x
```

---

## Quality Assurance and Testing

### Pre-Deployment Testing

1. **Dry-Run Release Process**
   - [ ] Run `scripts/release/start.sh` and verify it prompts for v2 version only
   - [ ] Check that `make echo-version` (or equivalent) returns v2 version
   - [ ] Verify `make draft-release` creates v2-only draft

2. **Build Verification**
   - [ ] Run `make build-all-platforms` and verify binaries contain v2 version
   - [ ] Check `jaeger-collector --version` and similar for v2 version string
   - [ ] Verify no v1 version information in built artifacts

3. **Docker Image Testing**
   - [ ] Build Docker images locally and verify tags are v2-only
   - [ ] Inspect image labels for version metadata
   - [ ] Test image functionality with v2 version

4. **CI Workflow Testing**
   - [ ] Trigger CI workflows on test branch
   - [ ] Verify only v2 artifacts are created
   - [ ] Check Docker Hub for correct v2 tags (if pushing to test registry)

5. **Documentation Review**
   - [ ] Review all updated docs for accuracy
   - [ ] Verify example commands work with v2 versions
   - [ ] Check that no outdated v1 references remain in user-facing docs

### Post-Deployment Validation

1. **First v2-Only Release**
   - [ ] Monitor release workflow execution
   - [ ] Verify v2 tag is created correctly
   - [ ] Check Docker Hub for v2 images published
   - [ ] Download and test published binaries

2. **Community Communication**
   - [ ] Announce v2-only release approach on mailing list
   - [ ] Update migration guides if necessary
   - [ ] Monitor for user issues or confusion

---

## Rollback Strategy

### If Issues Arise During Migration

1. **Before First Release:**
   - Revert commits to restore v1/v2 dual release logic
   - No production impact as release hasn't occurred

2. **After First v2-Only Release:**
   - If critical issues found, can manually create v1 tags from old commits if needed
   - Old release scripts still available in git history
   - Docker images can be manually built and pushed with v1 tags as fallback

3. **Recovery Commands:**
```bash
# Restore previous BuildInfo.mk:
git checkout HEAD~1 scripts/makefiles/BuildInfo.mk

# Manually create v1 tag if needed:
git tag v1.x.x <commit-sha> -s
git push upstream v1.x.x
```

### Communication Plan

- Notify team via Slack #jaeger-release channel before starting changes
- Document any issues encountered during first v2-only release
- Prepare rollback PR in advance if high-risk changes needed

---

## Timeline and Milestones

### Week 1-2: Preparation Phase
- [ ] Review and approve this checklist
- [ ] Assign owner for implementation
- [ ] Create tracking issue for implementation
- [ ] Set up test environment for dry-run releases

### Week 3-4: Implementation Phase
- [ ] Complete Critical Priority changes
- [ ] Complete High Priority changes
- [ ] Test release process end-to-end
- [ ] Complete Medium Priority changes

### Week 5: Validation and Release Phase
- [ ] Final QA and testing
- [ ] Perform first v2-only production release
- [ ] Monitor for issues
- [ ] Complete Low Priority changes

### 2026: Cleanup Phase
- [ ] Remove v1 Docker images from registry after deprecation
- [ ] Archive old release artifacts
- [ ] Final documentation cleanup
- [ ] Mark project complete

---

## Success Criteria

- ✅ All release scripts and build files use v2 versioning only
- ✅ CI/CD workflows publish v2 artifacts exclusively
- ✅ Documentation accurately reflects v2-only approach
- ✅ First v2-only release completes successfully
- ✅ No user-facing breaking changes or confusion
- ✅ Release process is simpler and faster than before
- ✅ Team understands new v2-only workflow

---

## Additional Notes

### Historical Context

The v1/v2 dual release approach was necessary during the transition period when Jaeger had both v1 (classic backend) and v2 (OTEL-based) architectures. With v2 now stable and v1 deprecated, maintaining dual releases adds unnecessary complexity.

### Dependencies

- Ensure v1 is officially deprecated before starting this work
- Coordinate with documentation team for updates
- Notify community of upcoming changes to release process

### Related Work

- This checklist focuses on build/release infrastructure only
- Separate work may be needed to update deployment docs
- Runtime configuration changes are out of scope

### Questions or Issues

For questions about this migration, contact:
- Owner: @yurishkuro
- Team channel: #jaeger-release
- Related issue: #7497

---

## Checklist Status

**Overall Progress:** 0% (0/43 files updated)

**By Priority:**
- Critical: 0/11 complete
- High: 0/6 complete  
- Medium: 0/5 complete
- Low: 0/4 complete
- Meta: 0/1 complete

---

*Last updated: 2025-11-12*  
*Next review: After first 5 critical files are completed*
