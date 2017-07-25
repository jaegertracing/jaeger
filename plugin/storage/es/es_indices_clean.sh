#!/bin/bash

function usage {
    >&2 echo "Error: $1"
    >&2 echo ""
    >&2 echo "Usage: $0 NUM_OF_DAYS HOSTNAME ... "
    >&2 echo ""
    >&2 echo "Specify a NUM_OF_DAYS that will delete indices that are older than the given NUM_OF_DAYS."
    >&2 echo "HOSTNAME ... specifies which ElasticSearch hosts to search and delete indices from."
    exit 1
}

if [[ "$1" == "" ]]; then
    usage "no number of days specified"
fi

pip install elasticsearch elasticsearch-curator
BASEDIR=$(dirname "$0")
cd $BASEDIR
python esCleaner.py "$@"
