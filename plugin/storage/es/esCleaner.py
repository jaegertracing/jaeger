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
        print('INDEX_DATE_SEPARATOR ... specifies index date separator.')
        print('ARCHIVE ... specifies whether to remove archive indices (only works for rollover) (default false).')
        print('ROLLOVER ... specifies whether to remove indices created by rollover (default false).')
        print('ES_USERNAME ... The username required by Elasticsearch.')
        print('ES_PASSWORD ... The password required by Elasticsearch.')
        print('ES_TLS ... enable TLS (default false).')
        print('ES_TLS_CA ... Path to TLS CA file.')
        print('ES_TLS_CERT ... Path to TLS certificate file.')
        print('ES_TLS_KEY ... Path to TLS key file.')
        print('ES_TLS_SKIP_HOST_VERIFY ... (insecure) Skip server\'s certificate chain and host name verification.')
        sys.exit(1)

    client = create_client(os.getenv("ES_USERNAME"), os.getenv("ES_PASSWORD"), str2bool(os.getenv("ES_TLS", 'false')), os.getenv("ES_TLS_CA"), os.getenv("ES_TLS_CERT"), os.getenv("ES_TLS_KEY"), str2bool(os.getenv("ES_TLS_SKIP_HOST_VERIFY", 'false')))
    ilo = curator.IndexList(client)
    empty_list(ilo, 'Elasticsearch has no indices')

    prefix = os.getenv("INDEX_PREFIX", '')
    if prefix != '':
        prefix += '-'
    separator = os.getenv("INDEX_DATE_SEPARATOR", '-')

    if str2bool(os.getenv("ARCHIVE", 'false')):
        filter_archive_indices_rollover(ilo, prefix)
    else:
        if str2bool(os.getenv("ROLLOVER", 'false')):
            filter_main_indices_rollover(ilo, prefix)
        else:
            filter_main_indices(ilo, prefix, separator)

    empty_list(ilo, 'No indices to delete')

    for index in ilo.working_list():
        print("Removing", index)
    timeout = int(os.getenv("TIMEOUT", TIMEOUT))
    delete_indices = curator.DeleteIndices(ilo, master_timeout=timeout)
    delete_indices.do_action()


def filter_main_indices(ilo, prefix, separator):
    date_regex = "\d{4}" + separator + "\d{2}" + separator + "\d{2}"
    time_string = "%Y" + separator + "%m" + separator + "%d"

    ilo.filter_by_regex(kind='regex', value=prefix + "jaeger-(span|service|dependencies)-" + date_regex)
    empty_list(ilo, "No indices to delete")
    # This excludes archive index as we use source='name'
    # source `creation_date` would include archive index
    ilo.filter_by_age(source='name', direction='older', timestring=time_string, unit='days', unit_count=int(sys.argv[1]))


def filter_main_indices_rollover(ilo, prefix):
    ilo.filter_by_regex(kind='regex', value=prefix + "jaeger-(span|service)-\d{6}")
    empty_list(ilo, "No indices to delete")
    # do not remove active write indices
    ilo.filter_by_alias(aliases=[prefix + 'jaeger-span-write'], exclude=True)
    empty_list(ilo, "No indices to delete")
    ilo.filter_by_alias(aliases=[prefix + 'jaeger-service-write'], exclude=True)
    empty_list(ilo, "No indices to delete")
    ilo.filter_by_age(source='creation_date', direction='older', unit='days', unit_count=int(sys.argv[1]))


def filter_archive_indices_rollover(ilo, prefix):
    # Remove only rollover archive indices
    # Do not remove active write archive index
    ilo.filter_by_regex(kind='regex', value=prefix + "jaeger-span-archive-\d{6}")
    empty_list(ilo, "No indices to delete")
    ilo.filter_by_alias(aliases=[prefix + 'jaeger-span-archive-write'], exclude=True)
    empty_list(ilo, "No indices to delete")
    ilo.filter_by_age(source='creation_date', direction='older', unit='days', unit_count=int(sys.argv[1]))


def empty_list(ilo, error_msg):
    try:
        ilo.empty_list_check()
    except curator.NoIndices:
        print(error_msg)
        sys.exit(0)


def str2bool(v):
    return v.lower() in ('true', '1')


def create_client(username, password, tls, ca, cert, key, skipHostVerify):
    context = ssl.create_default_context()
    if ca is not None:
        context = ssl.create_default_context(ssl.Purpose.SERVER_AUTH, cafile=ca)
    elif skipHostVerify:
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE
    if username is not None and password is not None:
        return elasticsearch.Elasticsearch(sys.argv[2:], http_auth=(username, password), ssl_context=context)
    elif tls:
        context.load_cert_chain(certfile=cert, keyfile=key)
        return elasticsearch.Elasticsearch(sys.argv[2:], ssl_context=context)
    else:
        return elasticsearch.Elasticsearch(sys.argv[2:], ssl_context=context)


if __name__ == "__main__":
    main()
