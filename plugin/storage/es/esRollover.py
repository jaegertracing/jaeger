#!/usr/bin/env python

import elasticsearch
import curator
import sys
import os
import ast
import logging

ARCHIVE_INDEX = 'jaeger-span-archive'
ROLLBACK_CONDITIONS = '{"max_age": "7d"}'
UNIT = 'days'
UNIT_COUNT = 7

def main():
    if len(sys.argv) != 3:
        print('USAGE: [INDEX_PREFIX=(default "")] [ARCHIVE=(default false)] [CONDITIONS=(default {})] [UNIT=(default {})] [UNIT_COUNT=(default {})] {} ACTION HOSTNAME[:PORT]'.format(ROLLBACK_CONDITIONS, UNIT, UNIT_COUNT, sys.argv[0]))
        print('ACTION ... one of:')
        print('\tinit - creates archive index and aliases')
        print('\trollover - rollover to new write index')
        print('\tlookback - removes old indices from read alias')
        print('HOSTNAME ... specifies which ElasticSearch hosts to search and delete indices from.')
        print('INDEX_PREFIX ... specifies index prefix.')
        print('rollover configuration:')
        print('\tCONDITIONS ... conditions used to rollover to a new write index e.g. \'{"max_age": "30d"}\'')
        print('lookback configuration:')
        print('\tUNIT ... used with lookback to remove indices from read alias e.g. ..., days, weeks, months, years')
        print('\tUNIT_COUNT ... count of UNITs')
        sys.exit(1)

    # TODO add rollover for main indices https://github.com/jaegertracing/jaeger/issues/1242
    if not str2bool(os.getenv('ARCHIVE', 'false')):
        print('Rollover for main indices is not supported')
        sys.exit(1)

    client = elasticsearch.Elasticsearch(sys.argv[2:])
    prefix = os.getenv('INDEX_PREFIX', '')
    if prefix != '':
        prefix += '-'
    write_alias = prefix + ARCHIVE_INDEX + '-write'
    read_alias = prefix + ARCHIVE_INDEX + '-read'

    action = sys.argv[1]
    if action == 'init':
        index = prefix + ARCHIVE_INDEX + '-000001'
        create_index(client, index)
        create_aliases(client, read_alias, index)
        create_aliases(client, write_alias, index)
    elif action == 'rollover':
        cond = ast.literal_eval(os.getenv('CONDITIONS', ROLLBACK_CONDITIONS))
        rollover(client, write_alias, read_alias, cond)
    elif action == 'lookback':
        read_alias_lookback(client, write_alias, read_alias, os.getenv('UNIT', UNIT), int(os.getenv('UNIT_COUNT', UNIT_COUNT)))
    else:
        print('Unrecognized action {}'.format(action))
        sys.exit(1)


def create_index(client, name):
    """
    Create archive index
    """
    print('Creating index {}'.format(name))
    create = curator.CreateIndex(client=client, name=name)
    create.do_action()


def create_aliases(client, alias_name, archive_index_name):
    """"
    Create read write aliases
    """
    ilo = curator.IndexList(client)
    ilo.filter_by_regex(kind='regex', value='^'+archive_index_name+'$')
    alias = curator.Alias(client=client, name=alias_name)
    for index in ilo.working_list():
        print("Adding index {} to alias {}".format(index, alias_name))
    alias.add(ilo)
    alias.do_action()


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


def empty_list(ilo, error_msg):
    try:
        ilo.empty_list_check()
    except curator.NoIndices:
        print(error_msg)
        sys.exit(0)


if __name__ == "__main__":
    logging.getLogger().setLevel(logging.DEBUG)
    main()
