#!/bin/bash
set -e

[[ $# -ge 2 ]] || { echo "Usage: $0 v1.x.x v2.x.x [--create-tags]"; exit 1; }

v1="$1"
v2="$2"
create_tags=false

if [[ "$3" == "--create-tags" ]]; then
    create_tags=true
fi

# Generate changelog 
make changelog > changelog_content.tmp

# Update CHANGELOG.md directly (no awk complexity)
current_date=$(date +"%Y-%m-%d")
echo "## $v1 / $v2 ($current_date)" > new_changelog.md
echo "" >> new_changelog.md  
cat changelog_content.tmp >> new_changelog.md
echo "" >> new_changelog.md
cat CHANGELOG.md >> new_changelog.md
mv new_changelog.md CHANGELOG.md

# Update jaeger-ui submodule
git submodule update --init jaeger-ui
cd jaeger-ui
git checkout main  
git pull
cd ..

# Git operations (only skip in dry-run)
if [[ "${DRY_RUN:-}" != "true" ]]; then
    if [[ "$create_tags" == "true" ]]; then
        # Tag creation mode
    git checkout main
    git pull --ff-only upstream main
        echo "About to create and push tags: $v1 and $v2"
        echo "This will run:"
        echo "  git tag $v1 -s -m \"Release $v1\""
        echo "  git tag $v2 -s -m \"Release $v2\""
        echo "  git push upstream $v1 $v2"
        echo ""
        read -p "Continue? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            git tag "$v1" -s -m "Release $v1"
            git tag "$v2" -s -m "Release $v2"
            git push upstream "$v1" "$v2"
            echo "Tags created and pushed successfully!"
        else
            echo "Tag creation cancelled."
        fi
    else
        # PR creation mode
        git checkout -b "prepare-release-$v1-$v2"
        git add CHANGELOG.md jaeger-ui
        git commit -m "Prepare release $v1 / $v2"
        git push origin "prepare-release-$v1-$v2"
        gh pr create --title "Prepare release $v1 / $v2" --label changelog:skip
        echo ""
        echo "PR created successfully. After merging the PR, run:"
        echo "bash ./scripts/release/prepare.sh $v1 $v2 --create-tags"
    fi
else
    echo "DRY RUN: Changes made locally. Would create PR for prepare-release-$v1-$v2"
fi

# Cleanup
rm -f changelog_content.tmp


