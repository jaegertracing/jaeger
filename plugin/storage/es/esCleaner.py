#!/usr/bin/env python

import elasticsearch
import curator
import sys
import os


def main():
    if len(sys.argv) == 1:
        print('USAGE: [TIMEOUT=(default 120)] [INDEX_PREFIX=(default "")] %s NUM_OF_DAYS HOSTNAME[:PORT] ...' % sys.argv[0])
        print('Specify a NUM_OF_DAYS that will delete indices that are older than the given NUM_OF_DAYS.')
        print('HOSTNAME ... specifies which ElasticSearch hosts to search and delete indices from.')
        print('INDEX_PREFIX ... specifies index prefix.')
        sys.exit(1)

    client = elasticsearch.Elasticsearch(sys.argv[2:])

    ilo = curator.IndexList(client)
    empty_list(ilo, 'ElasticSearch has no indices')

    prefix = os.getenv("INDEX_PREFIX", '')
    if prefix != '':
        prefix += ':'
    prefix += 'jaeger'

    ilo.filter_by_regex(kind='prefix', value=prefix)
    ilo.filter_by_age(source='name', direction='older', timestring='%Y-%m-%d', unit='days', unit_count=int(sys.argv[1]))
    empty_list(ilo, 'No indices to delete')

    for index in ilo.working_list():
        print("Removing", index)
    timeout = int(os.getenv("TIMEOUT", 120))
    delete_indices = curator.DeleteIndices(ilo, master_timeout=timeout)
    delete_indices.do_action()


def empty_list(ilo, error_msg):
    try:
        ilo.empty_list_check()
    except curator.NoIndices:
        print(error_msg)
        sys.exit(0)

if __name__ == "__main__":
    main()
