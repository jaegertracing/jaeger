#!/bin/bash
set -euxf -o pipefail

# stage-platform-files stages the different the platform ($1) into the package
# staging dir ($2). If you pass in a file extension ($3) it will be used when
# copying on the source
function stage-platform-files {
    local -r PLATFORM=$1
    local -r PACKAGE_STAGING_DIR=$2
    local -r FILE_EXTENSION=${3:-}

    cp ./cmd/all-in-one/all-in-one-$PLATFORM  $PACKAGE_STAGING_DIR/jaeger-all-in-one$FILE_EXTENSION
    cp ./cmd/agent/agent-$PLATFORM            $PACKAGE_STAGING_DIR/jaeger-agent$FILE_EXTENSION
    cp ./cmd/query/query-$PLATFORM            $PACKAGE_STAGING_DIR/jaeger-query$FILE_EXTENSION
    cp ./cmd/collector/collector-$PLATFORM    $PACKAGE_STAGING_DIR/jaeger-collector$FILE_EXTENSION
    cp ./cmd/ingester/ingester-$PLATFORM      $PACKAGE_STAGING_DIR/jaeger-ingester$FILE_EXTENSION
    cp ./examples/hotrod/hotrod-$PLATFORM     $PACKAGE_STAGING_DIR/example-hotrod$FILE_EXTENSION
}

# package pulls built files for the platform ($2) and compresses it using the compression ($1).
# If you pass in a file extension ($3) it will be look for binaries with that extension.
function package {
    local -r COMPRESSION=$1
    local -r PLATFORM=$2
    local -r FILE_EXTENSION=${3:-}
    local -r PACKAGE_NAME=jaeger-$VERSION-$PLATFORM
    local -r PACKAGE_STAGING_DIR=$PACKAGE_NAME

    if [ -d $PACKAGE_STAGING_DIR ]
    then
        rm -vrf "$PACKAGE_STAGING_DIR"
    fi
    mkdir $PACKAGE_STAGING_DIR
    stage-platform-files $PLATFORM $PACKAGE_STAGING_DIR $FILE_EXTENSION
    # Create a checksum file for all the files being packaged in the archive. Sorted by filename.
    find $PACKAGE_STAGING_DIR -type f -exec shasum -b -a 256 {} \; | sort -k2 | tee ./deploy/$PACKAGE_NAME.sha256sum.txt

    if [ "$COMPRESSION" == "zip" ]
    then
        local -r ARCHIVE_NAME="$PACKAGE_NAME.zip"
        echo "Packaging into $ARCHIVE_NAME:"
        zip -r ./deploy/$ARCHIVE_NAME $PACKAGE_STAGING_DIR
    else
        local -r ARCHIVE_NAME="$PACKAGE_NAME.tar.gz"
        echo "Packaging into $ARCHIVE_NAME:"
        tar --sort=name -czvf ./deploy/$ARCHIVE_NAME $PACKAGE_STAGING_DIR
    fi

    rm -rf $PACKAGE_STAGING_DIR
}

set -e

readonly VERSION="$(make echo-version | awk 'match($0, /([0-9]*\.[0-9]*\.[0-9]*)$/) { print substr($0, RSTART, RLENGTH) }')"
echo "Working on version: $VERSION"
if [ -z "$VERSION" ]
then
    # We want to halt if for some reason the version string is empty as this is an obvious error case
    >&2 echo 'Failed to detect a version string'
    exit 1
fi

# make needed directories
rm -rf deploy
mkdir deploy

package tar linux-amd64
package tar darwin-amd64
package tar darwin-arm64
package tar windows-amd64 .exe
package zip windows-amd64 .exe
package tar linux-s390x
package tar linux-arm64
package tar linux-ppc64le
# Create a checksum file for all non-checksum files in the deploy directory. Strips the leading 'deploy/' directory from filepaths. Sort by filename.
find deploy \( ! -name '*sha256sum.txt' \) -type f -exec shasum -b -a 256 {} \; | sed -r 's#(\w+\s+\*?)deploy/(.*)#\1\2#' | sort -k2 | tee ./deploy/jaeger-$VERSION.sha256sum.txt
