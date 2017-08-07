import elasticsearch
import curator
import logging
import sys
import os


def main():
    if len(sys.argv) == 1:
        print('U'
              'SAGE: [TIMEOUT=(default 120)] %s NUM_OF_DAYS HOSTNAME[:PORT] ...' % sys.argv[0])
        sys.exit(1)

    client = elasticsearch.Elasticsearch(sys.argv[2:])

    ilo = curator.IndexList(client)
    empty_list(ilo, 'ElasticSearch has no indices')
    ilo.filter_by_regex(kind='prefix', value='jaeger-')
    ilo.filter_by_age(source='name', direction='older', timestring='%Y-%m-%d', unit='days', unit_count=int(sys.argv[1]))
    empty_list(ilo, 'No indices to delete')

    for index in ilo.working_list():
        print "Removing", index
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
