import elasticsearch
import curator
import sys


def main():
    if len(sys.argv) == 1:
        print('USAGE: %s NUM_OF_DAYS HOSTNAME[:PORT] ...' % sys.argv[0])
        sys.exit(1)

    client = elasticsearch.Elasticsearch(sys.argv[2:])

    ilo = curator.IndexList(client)
    empty_list(ilo, 'ElasticSearch has no indices')
    ilo.filter_by_regex(kind='prefix', value='jaeger-')
    ilo.filter_by_age(source='name', direction='older', timestring='%Y-%m-%d', unit='days', unit_count=int(sys.argv[1]))
    empty_list(ilo, 'No indices to delete')

    delete_indices = curator.DeleteIndices(ilo)
    delete_indices.do_action()


def empty_list(ilo, error_msg):
    try:
        ilo.empty_list_check()
    except curator.NoIndices:
        print(error_msg)
        sys.exit(0)

if __name__ == "__main__":
    main()
