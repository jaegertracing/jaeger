#!/usr/bin/env python3

import curator
import elasticsearch
import os
import ssl
import sys

TIMEOUT=120

def main():
    if len(sys.argv) != 3:
        print('USAGE: [INDEX_PREFIX=(default "")] [ARCHIVE=(default false)] ... {} NUM_OF_DAYS http://HOSTNAME[:PORT]'.format(sys.argv[0]))
        print('NUM_OF_DAYS ... delete indices that are older than the given number of days.')
        print('HOSTNAME ... specifies which Elasticsearch hosts URL to search and delete indices from.')
        print('TIMEOUT ...  number of seconds to wait for master node response.'.format(TIMEOUT))
        print('INDEX_PREFIX ... specifies index prefix.')
        print('ARCHIVE ... specifies whether to remove archive indices (default false).')
        print('ES_USERNAME ... The username required by Elasticsearch.')
        print('ES_PASSWORD ... The password required by Elasticsearch.')
        print('ES_TLS ... enable TLS (default false).')
        print('ES_TLS_CA ... Path to TLS CA file.')
        print('ES_TLS_CERT ... Path to TLS certificate file.')
        print('ES_TLS_KEY ... Path to TLS key file.')
        sys.exit(1)

    username = os.getenv("ES_USERNAME")
    password = os.getenv("ES_PASSWORD")

    if username is not None and password is not None:
        client = elasticsearch.Elasticsearch(sys.argv[2:], http_auth=(username, password))
    elif str2bool(os.getenv("ES_TLS", 'false')):
        context = ssl.create_default_context(ssl.Purpose.SERVER_AUTH, cafile=os.getenv("ES_TLS_CA"))
        context.load_cert_chain(certfile=os.getenv("ES_TLS_CERT"), keyfile=os.getenv("ES_TLS_KEY"))
        client = elasticsearch.Elasticsearch(sys.argv[2:], ssl_context=context)
    else:
        client = elasticsearch.Elasticsearch(sys.argv[2:])

    ilo = curator.IndexList(client)
    empty_list(ilo, 'Elasticsearch has no indices')

    prefix = os.getenv("INDEX_PREFIX", '')
    if prefix != '':
        prefix += '-'

    if str2bool(os.getenv("ARCHIVE", 'false')):
        filter_archive_indices(ilo, prefix)
    else:
        filter_main_indices(ilo, prefix)

    empty_list(ilo, 'No indices to delete')

    for index in ilo.working_list():
        print("Removing", index)
    timeout = int(os.getenv("TIMEOUT", TIMEOUT))
    delete_indices = curator.DeleteIndices(ilo, master_timeout=timeout)
    delete_indices.do_action()


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
