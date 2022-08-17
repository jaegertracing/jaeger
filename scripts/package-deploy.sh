#!/bin/bash
# stage-file copies the file $1 with the specified path $2
# if no file exists it will abort
function stage-file {
    if [ ! -f $1 ]
    then
        echo "$1 does not exist. Aborting."
        exit 1
    fi
    echo "Copying $1 to $2"
    cp $1 $2
}

# stage-platform-files stages the different the platform ($1) into the package
# staging dir ($2). If you pass in a file extension ($3) it will be used when
# copying on the source
function stage-platform-files {
    local PLATFORM=$1
    local PACKAGE_STAGING_DIR=$2
    local FILE_EXTENSION=$3

    stage-file ./cmd/all-in-one/all-in-one-$PLATFORM  $PACKAGE_STAGING_DIR/jaeger-all-in-one$FILE_EXTENSION
    stage-file ./cmd/agent/agent-$PLATFORM            $PACKAGE_STAGING_DIR/jaeger-agent$FILE_EXTENSION
    stage-file ./cmd/query/query-$PLATFORM            $PACKAGE_STAGING_DIR/jaeger-query$FILE_EXTENSION
    stage-file ./cmd/collector/collector-$PLATFORM    $PACKAGE_STAGING_DIR/jaeger-collector$FILE_EXTENSION
    stage-file ./cmd/ingester/ingester-$PLATFORM      $PACKAGE_STAGING_DIR/jaeger-ingester$FILE_EXTENSION
    stage-file ./examples/hotrod/hotrod-$PLATFORM     $PACKAGE_STAGING_DIR/example-hotrod$FILE_EXTENSION
}

# package pulls built files for the platform ($2) and compresses it using the compression ($1).
# If you pass in a file extension ($3) it will be look for binaries with that extension.
function package {
    local COMPRESSION=$1
    local PLATFORM=$2
    local FILE_EXTENSION=$3
    local PACKAGE_NAME=jaeger-$VERSION-$PLATFORM
    local PACKAGE_STAGING_DIR=$PACKAGE_NAME

    if [ -d $PACKAGE_STAGING_DIR ]
    then
        rm -vrf "$PACKAGE_STAGING_DIR"
    fi
    mkdir $PACKAGE_STAGING_DIR
    stage-platform-files $PLATFORM $PACKAGE_STAGING_DIR $FILE_EXTENSION
    find $PACKAGE_STAGING_DIR -type f -exec sha256sum {} \; | sort -k2 | tee ./deploy/$PACKAGE_NAME.sha256sum.txt

    if [ "$COMPRESSION" == "zip" ]
    then
        local ARCHIVE_NAME="$PACKAGE_NAME.zip"
        echo "Packaging into $ARCHIVE_NAME:"
        zip -r ./deploy/$ARCHIVE_NAME $PACKAGE_STAGING_DIR
    else
        local ARCHIVE_NAME="$PACKAGE_NAME.tar.gz"
        echo "Packaging into $ARCHIVE_NAME:"
        tar --sort=name -czvf ./deploy/$ARCHIVE_NAME $PACKAGE_STAGING_DIR
    fi

    rm -rf $PACKAGE_STAGING_DIR
}

set -e

readonly VERSION="$(make echo-version | awk 'match($0, /([0-9]*\.[0-9]*\.[0-9]*)$/) { print substr($0, RSTART, RLENGTH) }')"
echo "Working on version: $VERSION"

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
find deploy \( ! -name '*sha256sum.txt' \) -type f -exec sha256sum {} \; | sed -r 's/(\w+\s+).*(jaeger)/\1\2/' | sort -k2 | tee ./deploy/jaeger-$VERSION.sha256sum.txt
