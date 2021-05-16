#!/usr/bin/env python3

import ast
import curator
import elasticsearch
import logging
import os
import requests
import ssl
import subprocess
import sys
import re
from requests.auth import HTTPBasicAuth

ARCHIVE_INDEX = 'jaeger-span-archive'
ROLLBACK_CONDITIONS = '{"max_age": "2d"}'
UNIT = 'days'
UNIT_COUNT = 2
SHARDS = 5
REPLICAS = 1
ILM_POLICY_NAME = 'jaeger-ilm-policy'

TIMEOUT=120

def main():
    if len(sys.argv) != 3:
        print(
            'USAGE: [INDEX_PREFIX=(default "")] [ARCHIVE=(default false)] ... {} ACTION http://HOSTNAME[:PORT]'.format(
                sys.argv[0]))
        print('ACTION ... one of:')
        print('\tinit - creates indices and aliases')
        print('\trollover - rollover to new write index')
        print('\tlookback - removes old indices from read alias')
        print('HOSTNAME ... specifies which Elasticsearch hosts URL to search and delete indices from.')
        print('INDEX_PREFIX ... specifies index prefix.')
        print('ARCHIVE ... handle archive indices (default false).')
        print('ES_USERNAME ... The username required by Elasticsearch.')
        print('ES_PASSWORD ... The password required by Elasticsearch.')
        print('ES_TLS ... enable TLS (default false).')
        print('ES_TLS_CA ... Path to TLS CA file.')
        print('ES_TLS_CERT ... Path to TLS certificate file.')
        print('ES_TLS_KEY ... Path to TLS key file.')
        print('ES_USE_ILM .. Use ILM to manage jaeger indices.')
        print('ES_ILM_POLICY_NAME .. The name of the ILM policy to use if ILM is active.')
        print('ES_TLS_SKIP_HOST_VERIFY ... (insecure) Skip server\'s certificate chain and host name verification.')
        print(
            'ES_VERSION ... The major Elasticsearch version. If not specified, the value will be auto-detected from Elasticsearch.')
        print('init configuration:')
        print('\tSHARDS ...  the number of shards per index in Elasticsearch (default {}).'.format(SHARDS))
        print('\tREPLICAS ... the number of replicas per index in Elasticsearch (default {}).'.format(REPLICAS))
        print('rollover configuration:')
        print('\tCONDITIONS ... conditions used to rollover to a new write index (default \'{}\'.'.format(
            ROLLBACK_CONDITIONS))
        print('lookback configuration:')
        print(
            '\tUNIT ... used with lookback to remove indices from read alias e.g. ..., days, weeks, months, years (default {}).'.format(
                UNIT))
        print('\tUNIT_COUNT ... count of UNITs (default {}).'.format(UNIT_COUNT))
        print('TIMEOUT ...  number of seconds to wait for master node response (default {}).'.format(TIMEOUT))
        sys.exit(1)

    timeout = int(os.getenv("TIMEOUT", TIMEOUT))

    client = create_client(os.getenv("ES_USERNAME"), os.getenv("ES_PASSWORD"), str2bool(os.getenv("ES_TLS", 'false')),
                           os.getenv("ES_TLS_CA"), os.getenv("ES_TLS_CERT"), os.getenv("ES_TLS_KEY"),
                           str2bool(os.getenv("ES_TLS_SKIP_HOST_VERIFY", 'false')), timeout)
    prefix = os.getenv('INDEX_PREFIX', '')
    if prefix != '':
        prefix += '-'

    action = sys.argv[1]

    if str2bool(os.getenv('ARCHIVE', 'false')):
        write_alias = prefix + ARCHIVE_INDEX + '-write'
        read_alias = prefix + ARCHIVE_INDEX + '-read'
        perform_action(action, client, write_alias, read_alias, prefix + 'jaeger-span-archive', 'jaeger-span', prefix)
    else:
        write_alias = prefix + 'jaeger-span-write'
        read_alias = prefix + 'jaeger-span-read'
        perform_action(action, client, write_alias, read_alias, prefix + 'jaeger-span', 'jaeger-span', prefix)
        write_alias = prefix + 'jaeger-service-write'
        read_alias = prefix + 'jaeger-service-read'
        perform_action(action, client, write_alias, read_alias, prefix + 'jaeger-service', 'jaeger-service', prefix)


def perform_action(action, client, write_alias, read_alias, index_to_rollover, template_name, prefix):
    if action == 'init':
        shards = os.getenv('SHARDS', SHARDS)
        replicas = os.getenv('REPLICAS', REPLICAS)
        esVersion = get_version(client)
        use_ilm = str2bool(os.getenv("ES_USE_ILM", 'false'))
        ilm_policy_name = os.getenv('ES_ILM_POLICY_NAME', ILM_POLICY_NAME)
        if esVersion == 7:
            if use_ilm:
                check_if_ilm_policy_exists(ilm_policy_name)
        else:
            if use_ilm:
                sys.exit("ILM is supported only for ES version 7+")
        create_index_template(fix_mapping(template_name, esVersion, shards, replicas, prefix.rstrip("-"), use_ilm, ilm_policy_name),
                              prefix + template_name)

        index = index_to_rollover + '-000001'
        create_index(client, index)
        if is_alias_empty(client, read_alias):
            create_aliases(client, read_alias, index, use_ilm)
        if is_alias_empty(client, write_alias):
            create_aliases(client, write_alias, index, use_ilm)
    elif action == 'rollover':
        cond = ast.literal_eval(os.getenv('CONDITIONS', ROLLBACK_CONDITIONS))
        rollover(client, write_alias, read_alias, cond)
    elif action == 'lookback':
        read_alias_lookback(client, write_alias, read_alias, os.getenv('UNIT', UNIT),
                            int(os.getenv('UNIT_COUNT', UNIT_COUNT)))
    else:
        print('Unrecognized action {}'.format(action))
        sys.exit(1)


def create_index_template(template, template_name):
    print('Creating index template {}'.format(template_name))
    headers = {'Content-Type': 'application/json'}
    s = get_request_session(os.getenv("ES_USERNAME"), os.getenv("ES_PASSWORD"), str2bool(os.getenv("ES_TLS", 'false')),
                            os.getenv("ES_TLS_CA"), os.getenv("ES_TLS_CERT"), os.getenv("ES_TLS_KEY"),
                            os.getenv("ES_TLS_SKIP_HOST_VERIFY", 'false'))
    r = s.put(sys.argv[2] + '/_template/' + template_name, headers=headers, data=template)
    print(r.text)
    r.raise_for_status()


def create_index(client, name):
    """
    Create archive index
    """
    print('Creating index {}'.format(name))
    create = curator.CreateIndex(client=client, name=name, ignore_existing=True)
    create.do_action()


def create_aliases(client, alias_name, archive_index_name, use_ilm):
    """"
    Create read write aliases
    """
    ilo = curator.IndexList(client)
    ilo.filter_by_regex(kind='regex', value='^' + archive_index_name + '$')
    for index in ilo.working_list():
        print("Adding index {} to alias {}".format(index, alias_name))
    if re.search(r'write', alias_name) and use_ilm:
        alias = curator.Alias(client=client, name=alias_name, extra_settings={'is_write_index': True})
    else:
        alias = curator.Alias(client=client, name=alias_name)
    alias.add(ilo)
    alias.do_action()


def is_alias_empty(client, alias_name):
    """"
    Checks whether alias is empty or not
    """
    ilo = curator.IndexList(client)
    ilo.filter_by_alias(aliases=alias_name)
    if len(ilo.working_list()) > 0:
        print("Alias {} is not empty. Not adding indices to it.".format(alias_name))
        return False
    return True


def rollover(client, write_alias, read_alias, conditions):
    """
    Rollover to new index and put it into read alias
    """
    print("Rollover {}, based on conditions {}".format(write_alias, conditions))
    roll = curator.Rollover(client=client, name=write_alias, conditions=conditions)
    roll.do_action()
    ilo = curator.IndexList(client)
    ilo.filter_by_alias(aliases=[write_alias])
    alias = curator.Alias(client=client, name=read_alias)
    for index in ilo.working_list():
        print("Adding index {} to alias {}".format(index, read_alias))
    alias.add(ilo)
    alias.do_action()


def read_alias_lookback(client, write_alias, read_alias, unit, unit_count):
    """
    This is used to mimic --es.max-span-age - The maximum lookback for spans in Elasticsearch
    by removing old indices from read alias
    """
    ilo = curator.IndexList(client)
    ilo.filter_by_alias(aliases=[read_alias])
    ilo.filter_by_alias(aliases=[write_alias], exclude=True)
    ilo.filter_by_age(source='creation_date', direction='older', unit=unit, unit_count=unit_count)
    empty_list(ilo, 'No indices to remove from alias {}'.format(read_alias))
    for index in ilo.working_list():
        print("Removing index {} from alias {}".format(index, read_alias))
    alias = curator.Alias(client=client, name=read_alias)
    alias.remove(ilo)
    alias.do_action()


def str2bool(v):
    return v.lower() in ('true', '1')


def fix_mapping(template_name, esVersion, shards, replicas, indexPrefix, use_ilm, ilm_policy_name):
    output = subprocess.Popen(['esmapping-generator', '--mapping', template_name, '--es-version', str(esVersion),
                               '--shards', str(shards), '--replicas',
                               str(replicas), '--index-prefix', indexPrefix,
                               '--use-ilm', str(use_ilm), '--ilm-policy-name', ilm_policy_name],
                              stdout=subprocess.PIPE,
                              stderr=subprocess.STDOUT)
    mapping, stderr = output.communicate()
    if output.returncode != 0:
        sys.exit(mapping)
    return mapping


def empty_list(ilo, error_msg):
    try:
        ilo.empty_list_check()
    except curator.NoIndices:
        print(error_msg)
        sys.exit(0)


def get_request_session(username, password, tls, ca, cert, key, skipHostVerify):
    session = requests.Session()
    if ca is not None:
        session.verify = ca
    elif skipHostVerify:
        session.verify = False
    if username is not None and password is not None:
        session.auth = HTTPBasicAuth(username, password)
    elif tls:
        session.verify = ca
        session.cert = (cert, key)
    return session


def get_version(client):
    esVersion = os.getenv('ES_VERSION')
    if esVersion is None or esVersion == '':
        esVersion = client.info()['version']['number'][0]
        print('Detected ElasticSearch Version {}'.format(esVersion))
        esVersion = int(esVersion)
    return esVersion


def create_client(username, password, tls, ca, cert, key, skipHostVerify, timeout):
    context = ssl.create_default_context()
    if ca is not None:
        context = ssl.create_default_context(ssl.Purpose.SERVER_AUTH, cafile=ca)
    elif skipHostVerify:
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE
    if username is not None and password is not None:
        return elasticsearch.Elasticsearch(sys.argv[2:], http_auth=(username, password), ssl_context=context, timeout=timeout)
    elif tls:
        context.load_cert_chain(certfile=cert, keyfile=key)
        return elasticsearch.Elasticsearch(sys.argv[2:], ssl_context=context, timeout=timeout)
    else:
        return elasticsearch.Elasticsearch(sys.argv[2:], ssl_context=context, timeout=timeout)


def check_if_ilm_policy_exists(ilm_policy):
    """"
    Checks whether ilm is created in Elasticsearch
    """
    s = get_request_session(os.getenv("ES_USERNAME"), os.getenv("ES_PASSWORD"), str2bool(os.getenv("ES_TLS", 'false')),
                            os.getenv("ES_TLS_CA"), os.getenv("ES_TLS_CERT"), os.getenv("ES_TLS_KEY"),
                            os.getenv("ES_TLS_SKIP_HOST_VERIFY", 'false'))
    r = s.get(sys.argv[2] + '/_ilm/policy/' + ilm_policy)
    if r.status_code != 200:
        sys.exit("ILM policy '{}' doesn't exist in Elasticsearch. Please create it and rerun init".format(ilm_policy))


if __name__ == "__main__":
    logging.getLogger().setLevel(logging.DEBUG)
    main()
