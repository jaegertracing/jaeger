#!/usr/bin/env python

import elasticsearch
import curator
import sys
import os


def main():
    if len(sys.argv) == 1:
        print('USAGE: [TIMEOUT=(default 120)] [INDEX_PREFIX=(default "")] [ARCHIVE=(default false)] {} NUM_OF_DAYS http://HOSTNAME[:PORT]'.format(sys.argv[0]))
        print('Specify a NUM_OF_DAYS that will delete indices that are older than the given NUM_OF_DAYS.')
        print('HOSTNAME ... specifies which Elasticsearch hosts URL to search and delete indices from.')
        print('INDEX_PREFIX ... specifies index prefix.')
        print('ARCHIVE ... specifies whether to remove archive indices. Use true or false')
        sys.exit(1)

    username = os.getenv("ES_USERNAME")
    password = os.getenv("ES_PASSWORD")

    if username is not None and password is not None:
        client = elasticsearch.Elasticsearch(sys.argv[2:], http_auth=(username, password))
    else:
        client = elasticsearch.Elasticsearch(sys.argv[2:])

    ilo = curator.IndexList(client)
    empty_list(ilo, 'ElasticSearch has no indices')

    prefix = os.getenv("INDEX_PREFIX", '')
    if prefix != '':
        prefix += '-'
    prefix += 'jaeger'

    if str2bool(os.getenv("ARCHIVE", 'false')):
        filter_archive_indices(ilo, prefix)
    else:
        filter_main_indices(ilo, prefix)

    empty_list(ilo, 'No indices to delete')

    for index in ilo.working_list():
        print("Removing", index)
    timeout = int(os.getenv("TIMEOUT", 120))
    delete_indices = curator.DeleteIndices(ilo, master_timeout=timeout)
    delete_indices.do_dry_run()


def filter_main_indices(ilo, prefix):
    ilo.filter_by_regex(kind='prefix', value=prefix + "jaeger")
    # This excludes archive index as we use source='name'
    # source `creation_date` would include archive index
    ilo.filter_by_age(source='name', direction='older', timestring='%Y-%m-%d', unit='days', unit_count=int(sys.argv[1]))


def filter_archive_indices(ilo, prefix):
    # Remove only archive indices when aliases are used
    # Do not remove active write archive index
    ilo.filter_by_alias(aliases=[prefix + 'jaeger-span-archive-write'], exclude=True)
    ilo.filter_by_alias(aliases=[prefix + 'jaeger-span-archive-read'])
    ilo.filter_by_age(source='creation_date', direction='older', unit='days', unit_count=int(sys.argv[1]))


def empty_list(ilo, error_msg):
    try:
        ilo.empty_list_check()
    except curator.NoIndices:
        print(error_msg)
        sys.exit(0)


def str2bool(v):
    return v.lower() in ('true', '1')


if __name__ == "__main__":
    main()
