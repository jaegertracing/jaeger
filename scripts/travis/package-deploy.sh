#!/bin/bash

# stage-file copies the file $1 with the specified path $2
# if no file exists it will silently continue
function stage-file {
    if [ -f $1 ]; then
        echo "Copying $1 to $2"
        cp $1 $2
    else
        echo "$1 does not exist. Continuing on."
    fi
}

# stage-platform-files stages the different the platform ($1) into the package
# staging dir ($2). If you pass in a file extension ($3) it will be used when 
# copying on both the target and the source
function stage-platform-files {
    local PLATFORM=$1
    local PACKAGE_STAGING_DIR=$2
    local FILE_EXTENSION=$3
    
    stage-file ./cmd/standalone/standalone-$PLATFORM$FILE_EXTENSION $PACKAGE_STAGING_DIR/jaeger-standalone$FILE_EXTENSION
    stage-file ./cmd/agent/agent-$PLATFORM$FILE_EXTENSION $PACKAGE_STAGING_DIR/jaeger-agent$FILE_EXTENSION
    stage-file ./cmd/query/query-$PLATFORM$FILE_EXTENSION $PACKAGE_STAGING_DIR/jaeger-query$FILE_EXTENSION
    stage-file ./cmd/collector/collector-$PLATFORM$FILE_EXTENSION $PACKAGE_STAGING_DIR/jaeger-collector$FILE_EXTENSION
}

# package pulls built files for the platform ($1). If you pass in a file 
# extension ($2) it will be used on the binaries
function package {
    local PLATFORM=$1
    local FILE_EXTENSION=$2

    local PACKAGE_STAGING_DIR=$DEPLOY_STAGING_DIR/$PLATFORM
    mkdir $PACKAGE_STAGING_DIR

    stage-platform-files $PLATFORM $PACKAGE_STAGING_DIR $FILE_EXTENSION

    local PACKAGE_FILES=$(ls -A $PACKAGE_STAGING_DIR/*) 2>/dev/null

    if [ "$PACKAGE_FILES" ]; then
        local ARCHIVE_NAME="jaeger-$VERSION-$PLATFORM-amd64.tar.gz"
        echo "Packaging the following files into $ARCHIVE_NAME:"
        echo $PACKAGE_FILES
        tar -czvf ./deploy/$ARCHIVE_NAME $PACKAGE_FILES
    else
        echo "Will not package or deploy $PLATFORM files as there are no files to package!"
    fi
}

# script start

DEPLOY_STAGING_DIR=./deploy-staging
VERSION="$(make echo-version | awk 'match($0, /([0-9]*\.[0-9]*\.[0-9]*)$/) { print substr($0, RSTART, RLENGTH) }')"
echo "Working on version: $VERSION"

# make needed directories
mkdir deploy
mkdir $DEPLOY_STAGING_DIR

# package linux
if [ "$LINUX" = true ]; then
    package linux
else
    echo "Skipping the packaging of linux binaries as \$LINUX was not true."
fi

# package darwin
if [ "$DARWIN" = true ]; then
    package darwin
else
    echo "Skipping the packaging of darwin binaries as \$DARWIN was not true."
fi

# package windows
if [ "$WINDOWS" = true ]; then
    package windows .exe
else
    echo "Skipping the packaging of windows binaries as \$WINDOWS was not true."
fi
