# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Script copied from https://github.com/christophebedard/dco-check/blob/master/dco_check/dco_check.py
#
# Copyright 2020 Christophe Bedard
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Check that all commits for a proposed change are signed off."""

import argparse
from collections import defaultdict
import json
import os
import re
import subprocess
import sys
from typing import Any
from typing import Dict
from typing import List
from typing import Optional
from typing import Tuple
from urllib import request


__version__ = '0.4.0'


DEFAULT_BRANCH = 'master'
DEFAULT_REMOTE = 'origin'
ENV_VAR_CHECK_MERGE_COMMITS = 'DCO_CHECK_CHECK_MERGE_COMMITS'
ENV_VAR_DEFAULT_BRANCH = 'DCO_CHECK_DEFAULT_BRANCH'
ENV_VAR_DEFAULT_BRANCH_FROM_REMOTE = 'DCO_CHECK_DEFAULT_BRANCH_FROM_REMOTE'
ENV_VAR_DEFAULT_REMOTE = 'DCO_CHECK_DEFAULT_REMOTE'
ENV_VAR_EXCLUDE_EMAILS = 'DCO_CHECK_EXCLUDE_EMAILS'
ENV_VAR_EXCLUDE_PATTERN = 'DCO_CHECK_EXCLUDE_PATTERN'
ENV_VAR_QUIET = 'DCO_CHECK_QUIET'
ENV_VAR_VERBOSE = 'DCO_CHECK_VERBOSE'
TRAILER_KEY_SIGNED_OFF_BY = 'Signed-off-by:'


class EnvDefaultOption(argparse.Action):
    """
    Action that uses an env var value as the default if it exists.

    Inspired by: https://stackoverflow.com/a/10551190/6476709
    """

    def __init__(
        self,
        env_var: str,
        default: Any,
        help: Optional[str] = None,  # noqa: A002
        **kwargs: Any,
    ) -> None:
        """Create an EnvDefaultOption."""
        # Set default to env var value if it exists
        if env_var in os.environ:
            default = os.environ[env_var]
        if help:  # pragma: no cover
            help += f' [env: {env_var}]'
        super(EnvDefaultOption, self).__init__(
            default=default,
            help=help,
            **kwargs,
        )

    def __call__(  # noqa: D102
        self,
        parser: argparse.ArgumentParser,
        namespace: argparse.Namespace,
        values: Any,
        option_string: Optional[str] = None,
    ) -> None:
        setattr(namespace, self.dest, values)


class EnvDefaultStoreTrue(argparse.Action):
    """
    Action similar to 'store_true' that uses an env var value as the default if it exists.

    Partly copied from arparse.{_StoreConstAction,_StoreTrueAction}.
    """

    def __init__(
        self,
        option_strings: str,
        dest: str,
        env_var: str,
        default: bool = False,
        help: Optional[str] = None,  # noqa: A002
    ) -> None:
        """Create an EnvDefaultStoreTrue."""
        # Set default value to true if the env var exists
        default = env_var in os.environ
        if help:  # pragma: no cover
            help += f' [env: {env_var} (any value to enable)]'
        super(EnvDefaultStoreTrue, self).__init__(
            option_strings=option_strings,
            dest=dest,
            nargs=0,
            const=True,
            default=default,
            required=False,
            help=help,
        )

    def __call__(  # noqa: D102
        self,
        parser: argparse.ArgumentParser,
        namespace: argparse.Namespace,
        values: Any,
        option_string: Optional[str] = None,
    ) -> None:
        setattr(namespace, self.dest, self.const)


def get_parser() -> argparse.ArgumentParser:
    """Get argument parser."""
    parser = argparse.ArgumentParser(
        description='Check that all commits of a proposed change have a DCO, i.e. are signed-off.',
    )
    default_branch_group = parser.add_mutually_exclusive_group()
    default_branch_group.add_argument(
        '-b', '--default-branch', metavar='BRANCH',
        action=EnvDefaultOption, env_var=ENV_VAR_DEFAULT_BRANCH,
        default=DEFAULT_BRANCH,
        help=(
            'default branch to use, if necessary (default: %(default)s)'
        ),
    )
    default_branch_group.add_argument(
        '--default-branch-from-remote',
        action=EnvDefaultStoreTrue, env_var=ENV_VAR_DEFAULT_BRANCH_FROM_REMOTE,
        default=False,
        help=(
            'get the default branch value from the remote (default: %(default)s)'
        ),
    )
    parser.add_argument(
        '-m', '--check-merge-commits',
        action=EnvDefaultStoreTrue, env_var=ENV_VAR_CHECK_MERGE_COMMITS,
        default=False,
        help=(
            'check sign-offs on merge commits as well (default: %(default)s)'
        ),
    )
    parser.add_argument(
        '-r', '--default-remote', metavar='REMOTE',
        action=EnvDefaultOption, env_var=ENV_VAR_DEFAULT_REMOTE,
        default=DEFAULT_REMOTE,
        help=(
            'default remote to use, if necessary (default: %(default)s)'
        ),
    )
    parser.add_argument(
        '-e', '--exclude-emails', metavar='EMAIL[,EMAIL]',
        action=EnvDefaultOption, env_var=ENV_VAR_EXCLUDE_EMAILS,
        default=None,
        help=(
            'exclude a comma-separated list of author emails from checks '
            '(commits with an author email matching one of these emails will be ignored)'
        ),
    )
    parser.add_argument(
        '-p', '--exclude-pattern', metavar='REGEX',
        action=EnvDefaultOption, env_var=ENV_VAR_EXCLUDE_PATTERN,
        default=None,
        help=(
            'exclude regular expresssion matched author emails from checks '
            '(commits with an author email matching regular expression pattern will be ignored)'
        ),
    )
    output_options_group = parser.add_mutually_exclusive_group()
    output_options_group.add_argument(
        '-q', '--quiet',
        action=EnvDefaultStoreTrue, env_var=ENV_VAR_QUIET,
        default=False,
        help=(
            'quiet mode (do not print anything; simply exit with 0 or non-zero) '
            '(default: %(default)s)'
        ),
    )
    output_options_group.add_argument(
        '-v', '--verbose',
        action=EnvDefaultStoreTrue, env_var=ENV_VAR_VERBOSE,
        default=False,
        help=(
            'verbose mode (print out more information) (default: %(default)s)'
        ),
    )
    parser.add_argument(
        '--version',
        action='version',
        help='show version number and exit',
        version=f'dco-check version {__version__}',
    )
    return parser


def parse_args(argv: Optional[List[str]] = None) -> argparse.Namespace:
    """
    Parse arguments.

    :param argv: the arguments to use, or `None` for sys.argv
    :return: the parsed arguments
    """
    return get_parser().parse_args(argv)


class Options:
    """Simple container and utilities for options."""

    def __init__(self, parser: argparse.ArgumentParser) -> None:
        """Create using default argument values."""
        self.check_merge_commits = parser.get_default('m')
        self.default_branch = parser.get_default('b')
        self.default_branch_from_remote = parser.get_default('default-branch-from-remote')
        self.default_remote = parser.get_default('r')
        self.exclude_emails = parser.get_default('e')
        self.exclude_pattern = parser.get_default('p')
        self.quiet = parser.get_default('q')
        self.verbose = parser.get_default('v')

    def set_options(self, args: argparse.Namespace) -> None:
        """Set options using parsed arguments."""
        self.check_merge_commits = args.check_merge_commits
        self.default_branch = args.default_branch
        self.default_branch_from_remote = args.default_branch_from_remote
        self.default_remote = args.default_remote
        # Split into list and filter out empty elements
        self.exclude_emails = list(filter(None, (args.exclude_emails or '').split(',')))
        self.exclude_pattern = (
            None if not args.exclude_pattern else re.compile(args.exclude_pattern)
        )
        self.quiet = args.quiet
        self.verbose = args.verbose
        # Shouldn't happen with a mutually exclusive group,
        # but can happen if one is set with an env var
        # and the other is set with an arg
        if self.quiet and self.verbose:
            # Similar message to what is printed when using args for both
            get_parser().print_usage()
            print("options '--quiet' and '--verbose' cannot both be true")
            sys.exit(1)
        if self.default_branch != DEFAULT_BRANCH and self.default_branch_from_remote:
            # Similar message to what is printed when using args for both
            get_parser().print_usage()
            print(
                "options '--default-branch' and '--default-branch-from-remote' cannot both be set"
            )
            sys.exit(1)

    def get_options(self) -> Dict[str, Any]:
        """Get all options as a dict."""
        return self.__dict__


options = Options(get_parser())


class Logger:
    """Simple logger to stdout which can be quiet or verbose."""

    def __init__(self, parser: argparse.ArgumentParser) -> None:
        """Create using default argument values."""
        self.__quiet = parser.get_default('q')
        self.__verbose = parser.get_default('v')

    def set_options(self, options: Options) -> None:
        """Set options using options object."""
        self.__quiet = options.quiet
        self.__verbose = options.verbose

    def print(self, msg: str = '', *args: Any, **kwargs: Any) -> None:  # noqa: A003
        """Print if not quiet."""
        if not self.__quiet:
            print(msg, *args, **kwargs)

    def verbose_print(self, msg: str = '', *args: Any, **kwargs: Any) -> None:
        """Print if verbose."""
        if self.__verbose:
            print(msg, *args, **kwargs)


logger = Logger(get_parser())


def run(
    command: List[str],
) -> Optional[str]:
    """
    Run command.

    :param command: the command list
    :return: the stdout output if the return code is 0, otherwise `None`
    """
    output = None
    try:
        env = os.environ.copy()
        if 'LANG' in env:
            del env['LANG']
        for key in list(env.keys()):
            if key.startswith('LC_'):
                del env[key]
        process = subprocess.Popen(
            command,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            env=env,
        )
        output_stdout, _ = process.communicate()
        if process.returncode != 0:
            logger.print(f'error: {output_stdout.decode("utf8")}')
        else:
            output = output_stdout.rstrip().decode('utf8').strip('\n')
    except subprocess.CalledProcessError as e:
        logger.print(f'error: {e.output.decode("utf8")}')
    return output


def is_valid_email(
    email: str,
) -> bool:
    """
    Check if email is valid.

    Simple regex checking for:
        <nonwhitespace string>@<nonwhitespace string>.<nonwhitespace string>

    :param email: the email address to check
    :return: true if email is valid, false otherwise
    """
    return bool(re.match(r'^\S+@\S+\.\S+', email))


def get_head_commit_hash() -> Optional[str]:
    """
    Get the hash of the HEAD commit.

    :return: the hash of the HEAD commit, or `None` if it failed
    """
    command = [
        'git',
        'rev-parse',
        '--verify',
        'HEAD',
    ]
    return run(command)


def get_common_ancestor_commit_hash(
    base_ref: str,
) -> Optional[str]:
    """
    Get the common ancestor commit of the current commit and a given reference.

    See: git merge-base --fork-point

    :param base_ref: the other reference
    :return: the common ancestor commit, or `None` if it failed
    """
    command = [
        'git',
        'merge-base',
        '--fork-point',
        base_ref,
    ]
    return run(command)


def fetch_branch(
    branch: str,
    remote: str = 'origin',
) -> int:
    """
    Fetch branch from remote.

    See: git fetch

    :param branch: the name of the branch
    :param remote: the name of the remote
    :return: zero for success, nonzero otherwise
    """
    command = [
        'git',
        'fetch',
        remote,
        branch,
    ]
    # We don't want the output
    return 0 if run(command) is not None else 1


def get_default_branch_from_remote(
    remote: str,
) -> Optional[str]:
    """
    Get default branch from remote.

    :param remote: the remote name
    :return: the default branch, or None if it failed
    """
    # https://stackoverflow.com/questions/28666357/git-how-to-get-default-branch#comment92366240_50056710  # noqa: E501
    #   $ git remote show origin
    cmd = ['git', 'remote', 'show', remote]
    result = run(cmd)
    if not result:
        return None
    result_lines = result.split('\n')
    branch = None
    for result_line in result_lines:
        # There is a two-space indentation
        match = re.match('  HEAD branch: (.*)', result_line)
        if match:
            branch = match[1]
            break
    return branch


def get_commits_data(
    base: str,
    head: str,
    ignore_merge_commits: bool = True,
) -> Optional[str]:
    """
    Get data (full sha & commit body) for commits in a range.

    The range excludes the 'before' commit, e.g. ]base, head]
    The output data contains data for individual commits, separated by special characters:
       * 1st line: full commit sha
       * 2nd line: author name and email
       * 3rd line: commit title (subject)
       * subsequent lines: commit body (which excludes the commit title line)
       * record separator (0x1e)

    :param base: the sha of the commit just before the start of the range
    :param head: the sha of the last commit of the range
    :param ignore_merge_commits: whether to ignore merge commits
    :return: the data, or `None` if it failed
    """
    command = [
        'git',
        'log',
        f'{base}..{head}',
        '--pretty=%H%n%an <%ae>%n%s%n%-b%x1e',
    ]
    if ignore_merge_commits:
        command += ['--no-merges']
    return run(command)


def split_commits_data(
    commits_data: str,
    commits_sep: str = '\x1e',
) -> List[str]:
    """
    Split data into individual commits using a separator.

    :param commits_data: the full data to be split
    :param commits_sep: the string which separates individual commits
    :return: the list of data for each individual commit
    """
    # Remove leading/trailing newlines
    commits_data = commits_data.strip('\n')
    # Split in individual commits and remove leading/trailing newlines
    individual_commits = [
        single_output.strip('\n') for single_output in commits_data.split(commits_sep)
    ]
    # Filter out empty elements
    individual_commits = list(filter(None, individual_commits))
    return individual_commits


def extract_name_and_email(
    name_and_email: str,
) -> Optional[Tuple[str, str]]:
    """
    Extract a name and an email from a 'name <email>' string.

    :param name_and_email: the name and email string
    :return: the extracted (name, email) tuple, or `None` if it failed
    """
    match = re.search('(.*) <(.*)>', name_and_email)
    if not match:
        return None
    return match.group(1), match.group(2)


def format_name_and_email(
    name: Optional[str],
    email: Optional[str],
) -> str:
    """
    Format a name and a email into a 'name <email>' string.

    :param name: the name, or `None` if N/A
    :param email: the email, or `None` if N/A
    :return: the formatted string
    """
    return f"{name or 'N/A'} <{email or 'N/A'}>"


def get_env_var(
    env_var: str,
    print_if_not_found: bool = True,
    default: Optional[str] = None,
) -> Optional[str]:
    """
    Get the value of an environment variable.

    :param env_var: the environment variable name/key
    :param print_if_not_found: whether to print if the environment variable could not be found
    :param default: the value to use if the environment variable could not be found
    :return: the environment variable value, or `None` if not found and no default value was given
    """
    value = os.environ.get(env_var, None)
    if value is None:
        if default is not None:
            if print_if_not_found:
                logger.print(
                    f"could not get environment variable: '{env_var}'; "
                    f"using value default value: '{default}'"
                )
            value = default
        elif print_if_not_found:
            logger.print(f"could not get environment variable: '{env_var}'")
    return value


class CommitInfo:
    """Container for all necessary commit information."""

    def __init__(
        self,
        commit_hash: str,
        title: str,
        body: List[str],
        author_name: Optional[str],
        author_email: Optional[str],
        is_merge_commit: bool = False,
    ) -> None:
        """Create a CommitInfo object."""
        self.hash = commit_hash
        self.title = title
        self.body = body
        self.author_name = author_name
        self.author_email = author_email
        self.is_merge_commit = is_merge_commit


class CommitDataRetriever:
    """
    Abstract commit data retriever.

    It first provides a method to check whether it applies to the current setup or not.
    It also provides other methods to get commits to be checked.
    These should not be called if it doesn't apply.
    """

    def name(self) -> str:
        """Get a name that represents this retriever."""
        raise NotImplementedError  # pragma: no cover

    def applies(self) -> bool:
        """Check if this retriever applies, i.e. can provide commit data."""
        raise NotImplementedError  # pragma: no cover

    def get_commit_range(self) -> Optional[Tuple[str, str]]:
        """
        Get the range of commits to be checked: (last commit that was checked, latest commit).

        The range excludes the first commit, e.g. ]first commit, second commit]

        :return the (last commit that was checked, latest commit) tuple, or `None` if it failed
        """
        raise NotImplementedError  # pragma: no cover

    def get_commits(self, base: str, head: str, **kwargs: Any) -> Optional[List[CommitInfo]]:
        """Get commit data."""
        raise NotImplementedError  # pragma: no cover


class GitRetriever(CommitDataRetriever):
    """Implementation for any git repository."""

    def name(self) -> str:  # noqa: D102
        return 'git (default)'

    def applies(self) -> bool:  # noqa: D102
        # Unless we only have access to a partial commit history
        return True

    def get_commit_range(self) -> Optional[Tuple[str, str]]:  # noqa: D102
        default_branch = options.default_branch
        logger.verbose_print(f"\tusing default branch '{default_branch}'")
        commit_hash_base = get_common_ancestor_commit_hash(default_branch)
        if not commit_hash_base:
            return None
        commit_hash_head = get_head_commit_hash()
        if not commit_hash_head:
            return None
        return commit_hash_base, commit_hash_head

    def get_commits(  # noqa: D102
        self,
        base: str,
        head: str,
        check_merge_commits: bool = False,
        **kwargs: Any,
    ) -> Optional[List[CommitInfo]]:
        ignore_merge_commits = not check_merge_commits
        commits_data = get_commits_data(base, head, ignore_merge_commits=ignore_merge_commits)
        commits: List[CommitInfo] = []
        if commits_data is None:
            return commits
        individual_commits = split_commits_data(commits_data)
        for commit_data in individual_commits:
            commit_lines = commit_data.split('\n')
            commit_hash = commit_lines[0]
            commit_author_data = commit_lines[1]
            commit_title = commit_lines[2]
            commit_body = commit_lines[3:]
            author_result = extract_name_and_email(commit_author_data)
            author_name, author_email = None, None
            if author_result:
                author_name, author_email = author_result
            # There won't be any merge commits at this point
            is_merge_commit = False
            commits.append(
                CommitInfo(
                    commit_hash,
                    commit_title,
                    commit_body,
                    author_name,
                    author_email,
                    is_merge_commit,
                )
            )
        return commits


class GitLabRetriever(GitRetriever):
    """Implementation for GitLab CI."""

    def name(self) -> str:  # noqa: D102
        return 'GitLab'

    def applies(self) -> bool:  # noqa: D102
        return get_env_var('GITLAB_CI', print_if_not_found=False) is not None

    def get_commit_range(self) -> Optional[Tuple[str, str]]:  # noqa: D102
        # See: https://docs.gitlab.com/ee/ci/variables/predefined_variables.html
        default_branch = get_env_var('CI_DEFAULT_BRANCH', default=options.default_branch)

        commit_hash_head = get_env_var('CI_COMMIT_SHA')
        if not commit_hash_head:
            return None

        current_branch = get_env_var('CI_COMMIT_BRANCH')
        if get_env_var('CI_PIPELINE_SOURCE') == 'schedule':
            # Do not check scheduled pipelines
            logger.verbose_print("\ton scheduled pipeline: won't check commits")
            return commit_hash_head, commit_hash_head
        elif current_branch == default_branch:
            # If we're on the default branch, just test new commits
            logger.verbose_print(
                f"\ton default branch '{current_branch}': "
                'will check new commits'
            )
            commit_hash_base = get_env_var('CI_COMMIT_BEFORE_SHA')
            if commit_hash_base == '0000000000000000000000000000000000000000':
                logger.verbose_print('\tfound no new commits')
                return commit_hash_head, commit_hash_head
            if not commit_hash_base:
                return None
            return commit_hash_base, commit_hash_head
        elif get_env_var('CI_MERGE_REQUEST_ID', print_if_not_found=False):
            # Get merge request target branch
            target_branch = get_env_var('CI_MERGE_REQUEST_TARGET_BRANCH_NAME')
            if not target_branch:
                return None
            logger.verbose_print(
                f"\ton merge request branch '{current_branch}': "
                f"will check new commits off of target branch '{target_branch}'"
            )
            target_branch_sha = get_env_var('CI_MERGE_REQUEST_TARGET_BRANCH_SHA')
            if not target_branch_sha:
                return None
            return target_branch_sha, commit_hash_head
        elif get_env_var('CI_EXTERNAL_PULL_REQUEST_IID', print_if_not_found=False):
            # Get external merge request target branch
            target_branch = get_env_var('CI_EXTERNAL_PULL_REQUEST_TARGET_BRANCH_NAME')
            if not target_branch:
                return None
            logger.verbose_print(
                f"\ton merge request branch '{current_branch}': "
                f"will check new commits off of target branch '{target_branch}'"
            )
            target_branch_sha = get_env_var('CI_EXTERNAL_PULL_REQUEST_TARGET_BRANCH_SHA')
            if not target_branch_sha:
                return None
            return target_branch_sha, commit_hash_head
        else:
            if not default_branch:
                return None
            # Otherwise test all commits off of the default branch
            logger.verbose_print(
                f"\ton branch '{current_branch}': "
                f"will check forked commits off of default branch '{default_branch}'"
            )
            # Fetch default branch
            remote = options.default_remote
            if 0 != fetch_branch(default_branch, remote):
                logger.print(f"failed to fetch '{default_branch}' from remote '{remote}'")
                return None
            # Use remote default branch ref
            remote_branch_ref = remote + '/' + default_branch
            commit_hash_base = get_common_ancestor_commit_hash(remote_branch_ref)
            if not commit_hash_base:
                return None
            return commit_hash_base, commit_hash_head


class CircleCiRetriever(GitRetriever):
    """Implementation for CircleCI."""

    def name(self) -> str:  # noqa: D102
        return 'CircleCI'

    def applies(self) -> bool:  # noqa: D102
        return get_env_var('CIRCLECI', print_if_not_found=False) is not None

    def get_commit_range(self) -> Optional[Tuple[str, str]]:  # noqa: D102
        # See: https://circleci.com/docs/2.0/env-vars/#built-in-environment-variables
        default_branch = options.default_branch

        commit_hash_head = get_env_var('CIRCLE_SHA1')
        if not commit_hash_head:
            return None

        # Check if base revision is provided to the environment, e.g.
        #   environment:
        #     CIRCLE_BASE_REVISION: << pipeline.git.base_revision >>
        # See:
        #   https://circleci.com/docs/2.0/pipeline-variables/
        #   https://circleci.com/docs/2.0/env-vars/#built-in-environment-variables
        base_revision = get_env_var('CIRCLE_BASE_REVISION', print_if_not_found=False)
        if base_revision:
            # For PRs, this is the commit of the base branch,
            # and, for pushes to a branch, this is the commit before the new commits
            logger.verbose_print(
                f"\tchecking commits off of base revision '{base_revision}'"
            )
            return base_revision, commit_hash_head
        else:
            current_branch = get_env_var('CIRCLE_BRANCH')
            if not current_branch:
                return None
            # Test all commits off of the default branch
            logger.verbose_print(
                f"\ton branch '{current_branch}': "
                f"will check forked commits off of default branch '{default_branch}'"
            )
            # Fetch default branch
            remote = options.default_remote
            if 0 != fetch_branch(default_branch, remote):
                logger.print(f"failed to fetch '{default_branch}' from remote '{remote}'")
                return None
            # Use remote default branch ref
            remote_branch_ref = remote + '/' + default_branch
            commit_hash_base = get_common_ancestor_commit_hash(remote_branch_ref)
            if not commit_hash_base:
                return None
            return commit_hash_base, commit_hash_head


class AzurePipelinesRetriever(GitRetriever):
    """Implementation for Azure Pipelines."""

    def name(self) -> str:  # noqa: D102
        return 'Azure Pipelines'

    def applies(self) -> bool:  # noqa: D102
        return get_env_var('TF_BUILD', print_if_not_found=False) is not None

    def get_commit_range(self) -> Optional[Tuple[str, str]]:  # noqa: D102
        # See: https://docs.microsoft.com/en-us/azure/devops/pipelines/build/variables?view=azure-devops&tabs=yaml#build-variables  # noqa: E501
        commit_hash_head = get_env_var('BUILD_SOURCEVERSION')
        if not commit_hash_head:
            return None
        current_branch = get_env_var('BUILD_SOURCEBRANCHNAME')
        if not current_branch:
            return None

        base_branch = None
        # Check if pull request
        is_pull_request = get_env_var(
            'SYSTEM_PULLREQUEST_PULLREQUESTID',
            print_if_not_found=False,
        )
        if is_pull_request:
            # Test all commits off of the target branch
            target_branch = get_env_var('SYSTEM_PULLREQUEST_TARGETBRANCH')
            if not target_branch:
                return None
            logger.verbose_print(
                f"\ton pull request branch '{current_branch}': "
                f"will check forked commits off of target branch '{target_branch}'"
            )
            base_branch = target_branch
        else:
            # Test all commits off of the default branch
            default_branch = options.default_branch
            logger.verbose_print(
                f"\ton branch '{current_branch}': "
                f"will check forked commits off of default branch '{default_branch}'"
            )
            base_branch = default_branch
        # Fetch base branch
        assert base_branch
        remote = options.default_remote
        if 0 != fetch_branch(base_branch, remote):
            logger.print(f"failed to fetch '{base_branch}' from remote '{remote}'")
            return None
        # Use remote default branch ref
        remote_branch_ref = remote + '/' + base_branch
        commit_hash_base = get_common_ancestor_commit_hash(remote_branch_ref)
        if not commit_hash_base:
            return None
        return commit_hash_base, commit_hash_head


class AppVeyorRetriever(GitRetriever):
    """Implementation for AppVeyor."""

    def name(self) -> str:  # noqa: D102
        return 'AppVeyor'

    def applies(self) -> bool:  # noqa: D102
        return get_env_var('APPVEYOR', print_if_not_found=False) is not None

    def get_commit_range(self) -> Optional[Tuple[str, str]]:  # noqa: D102
        # See: https://www.appveyor.com/docs/environment-variables/
        default_branch = options.default_branch

        commit_hash_head = get_env_var('APPVEYOR_REPO_COMMIT')
        if not commit_hash_head:
            commit_hash_head = get_head_commit_hash()
            if not commit_hash_head:
                return None

        branch = get_env_var('APPVEYOR_REPO_BRANCH')
        if not branch:
            return None

        # Check if pull request
        if get_env_var('APPVEYOR_PULL_REQUEST_NUMBER', print_if_not_found=False):
            current_branch = get_env_var('APPVEYOR_PULL_REQUEST_HEAD_REPO_BRANCH')
            if not current_branch:
                return None
            target_branch = branch
            logger.verbose_print(
                f"\ton pull request branch '{current_branch}': "
                f"will check commits off of target branch '{target_branch}'"
            )
            commit_hash_head = get_env_var('APPVEYOR_PULL_REQUEST_HEAD_COMMIT') or commit_hash_head
            if not commit_hash_head:
                return None
            commit_hash_base = get_common_ancestor_commit_hash(target_branch)
            if not commit_hash_base:
                return None
            return commit_hash_base, commit_hash_head
        else:
            # Otherwise test all commits off of the default branch
            current_branch = branch
            logger.verbose_print(
                f"\ton branch '{current_branch}': "
                f"will check forked commits off of default branch '{default_branch}'"
            )
            commit_hash_base = get_common_ancestor_commit_hash(default_branch)
            if not commit_hash_base:
                return None
            return commit_hash_base, commit_hash_head


class GitHubRetriever(CommitDataRetriever):
    """Implementation for GitHub CI."""

    def name(self) -> str:  # noqa: D102
        return 'GitHub CI'

    def applies(self) -> bool:  # noqa: D102
        return get_env_var('GITHUB_ACTIONS', print_if_not_found=False) == 'true'

    def get_commit_range(self) -> Optional[Tuple[str, str]]:  # noqa: D102
        # See: https://docs.gitlab.com/ee/ci/variables/predefined_variables.html
        self.github_token = get_env_var('GITHUB_TOKEN')
        if not self.github_token:
            logger.print('Did you forget to include this in your workflow config?')
            logger.print('\n\tenv:\n\t\tGITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}')
            return None

        # See: https://help.github.com/en/actions/configuring-and-managing-workflows/using-environment-variables  # noqa: E501
        event_payload_path = get_env_var('GITHUB_EVENT_PATH')
        if not event_payload_path:
            return None
        f = open(event_payload_path)
        self.event_payload = json.load(f)
        f.close()

        # Get base & head commits depending on the workflow event type
        event_name = get_env_var('GITHUB_EVENT_NAME')
        if not event_name:
            return None
        commit_hash_base = None
        commit_hash_head = None
        if event_name in ('pull_request', 'pull_request_target'):
            # See: https://developer.github.com/v3/activity/events/types/#pullrequestevent
            commit_hash_base = self.event_payload['pull_request']['base']['sha']
            commit_hash_head = self.event_payload['pull_request']['head']['sha']
            commit_branch_base = self.event_payload['pull_request']['base']['ref']
            commit_branch_head = self.event_payload['pull_request']['head']['ref']
            logger.verbose_print(
                f"\ton pull request branch '{commit_branch_head}': "
                f"will check commits off of base branch '{commit_branch_base}'"
            )
        elif event_name == 'push':
            # See: https://developer.github.com/v3/activity/events/types/#pushevent
            created = self.event_payload['created']
            if created:
                # If the branch was just created, there won't be a 'before' commit,
                # therefore just get the first commit in the new branch and append '^'
                # to get the commit before that one
                commits = self.event_payload['commits']
                # TODO check len(commits),
                # it's probably 0 when pushing a new branch that is based on an existing one
                commit_hash_base = commits[0]['id'] + '^'
            else:
                commit_hash_base = self.event_payload['before']
            commit_hash_head = self.event_payload['head_commit']['id']
        else:  # pragma: no cover
            logger.print('Unknown workflow event:', event_name)
            return None
        return commit_hash_base, commit_hash_head

    def get_commits(  # noqa: D102
        self,
        base: str,
        head: str,
        **kwargs: Any,
    ) -> Optional[List[CommitInfo]]:
        # Request commit data
        compare_url_template = self.event_payload['repository']['compare_url']
        compare_url = compare_url_template.format(base=base, head=head)
        req = request.Request(compare_url, headers={
            'User-Agent': 'dco_check',
            'Authorization': 'token ' + (self.github_token or ''),
        })
        response = request.urlopen(req)
        if 200 != response.getcode():  # pragma: no cover
            from pprint import pformat
            logger.print('Request failed: compare_url')
            logger.print('reponse:', pformat(response.read().decode()))
            return None

        # Extract data
        response_json = json.load(response)
        commits = []
        for commit in response_json['commits']:
            commit_hash = commit['sha']
            message = commit['commit']['message'].split('\n')
            message = list(filter(None, message))
            commit_title = message[0]
            commit_body = message[1:]
            author_name = commit['commit']['author']['name']
            author_email = commit['commit']['author']['email']
            is_merge_commit = len(commit['parents']) > 1
            commits.append(
                CommitInfo(
                    commit_hash,
                    commit_title,
                    commit_body,
                    author_name,
                    author_email,
                    is_merge_commit,
                )
            )
        return commits


def process_commits(
    commits: List[CommitInfo],
    check_merge_commits: bool,
) -> Dict[str, List[str]]:
    """
    Process commit information to detect DCO infractions.

    :param commits: the list of commit info
    :param check_merge_commits: true to check merge commits, false otherwise
    :return: the infractions as a dict {commit sha, infraction explanation}
    """
    infractions: Dict[str, List[str]] = defaultdict(list)
    for commit in commits:
        # Skip this commit if it is a merge commit and the
        # option for checking merge commits is not enabled
        if commit.is_merge_commit and not check_merge_commits:
            logger.verbose_print('\t' + 'ignoring merge commit:', commit.hash)
            logger.verbose_print()
            continue

        logger.verbose_print(
            '\t' + commit.hash + (' (merge commit)' if commit.is_merge_commit else '')
        )
        logger.verbose_print('\t' + format_name_and_email(commit.author_name, commit.author_email))
        logger.verbose_print('\t' + commit.title)
        logger.verbose_print('\t' + '\n\t'.join(commit.body))

        # Check author name and email
        if any(not d for d in [commit.author_name, commit.author_email]):
            infractions[commit.hash].append(
                f'could not extract author data for commit: {commit.hash}'
            )
            continue

        # Check if the commit should be ignored because of the commit author email
        if options.exclude_emails and commit.author_email in options.exclude_emails:
            logger.verbose_print('\t\texcluding commit since author email is in exclude list')
            logger.verbose_print()
            continue

        # Check if the commit should be ignored because of the commit author email pattern
        if commit.author_email and options.exclude_pattern:
            if options.exclude_pattern.search(commit.author_email):
                logger.verbose_print('\t\texcluding commit since author email is matched by')
                logger.verbose_print('\t\tpattern')
                logger.verbose_print()
                continue

        # Extract sign-off data
        sign_offs = [
            body_line.replace(TRAILER_KEY_SIGNED_OFF_BY, '').strip(' ')
            for body_line in commit.body
            if body_line.startswith(TRAILER_KEY_SIGNED_OFF_BY)
        ]

        # Check that there is at least one sign-off right away
        if len(sign_offs) == 0:
            infractions[commit.hash].append('no sign-off found')
            continue

        # Extract sign off information
        sign_offs_name_email: List[Tuple[str, str]] = []
        for sign_off in sign_offs:
            sign_off_result = extract_name_and_email(sign_off)
            if not sign_off_result:
                continue
            name, email = sign_off_result
            logger.verbose_print(f'\t\tfound sign-off: {format_name_and_email(name, email)}')
            if not is_valid_email(email):
                infractions[commit.hash].append(f'invalid email: {email}')
            else:
                sign_offs_name_email.append((name, email.lower()))

        # Check that author is in the sign-offs
        if not (commit.author_name, commit.author_email.lower()) in sign_offs_name_email:
            infractions[commit.hash].append(
                'sign-off not found for commit author: '
                f'{commit.author_name} {commit.author_email}; found: {sign_offs}'
            )

        # Separator between commits
        logger.verbose_print()

    return infractions


def check_infractions(
    infractions: Dict[str, List[str]],
) -> int:
    """
    Check infractions.

    :param infractions: the infractions dict {commit sha, infraction explanation}
    :return: 0 if no infractions, non-zero otherwise
    """
    if len(infractions) > 0:
        logger.print('Missing sign-off(s):')
        logger.print()
        for commit_sha, commit_infractions in infractions.items():
            logger.print('\t' + commit_sha)
            for commit_infraction in commit_infractions:
                logger.print('\t\t' + commit_infraction)
        return 1
    logger.print('All good!')
    return 0


def main(argv: Optional[List[str]] = None) -> int:
    """
    Entrypoint.

    :param argv: the arguments to use, or `None` for sys.argv
    :return: 0 if successful, non-zero otherwise
    """
    args = parse_args(argv)
    options.set_options(args)
    logger.set_options(options)

    # Print options
    if options.verbose:
        logger.verbose_print('Options:')
        for name, value in options.get_options().items():
            logger.verbose_print(f'\t{name}: {str(value)}')
        logger.verbose_print()

    # Detect CI
    # Use first one that applies
    retrievers = [
        GitLabRetriever,
        GitHubRetriever,
        AzurePipelinesRetriever,
        AppVeyorRetriever,
        CircleCiRetriever,
        GitRetriever,
    ]
    commit_retriever = None
    for retriever_cls in retrievers:
        retriever = retriever_cls()
        if retriever.applies():
            commit_retriever = retriever
            break
    if not commit_retriever:
        logger.print('Could not find an applicable GitRetriever')
        return 1
    logger.print('Detected:', commit_retriever.name())

    # Get default branch from remote if enabled
    if options.default_branch_from_remote:
        remote_default_branch = get_default_branch_from_remote(options.default_remote)
        if not remote_default_branch:
            logger.print('Could not get default branch from remote')
            return 1
        options.default_branch = remote_default_branch
        logger.print(f"\tgot default branch '{remote_default_branch}' from remote")

    # Get range of commits
    commit_range = commit_retriever.get_commit_range()
    if not commit_range:
        return 1
    commit_hash_base, commit_hash_head = commit_range

    logger.print()
    # Return success now if base == head
    if commit_hash_base == commit_hash_head:
        logger.print('No commits to check')
        return 0

    logger.print(f'Checking commits: {commit_hash_base}..{commit_hash_head}')
    logger.print()

    # Get commits
    commits = commit_retriever.get_commits(
        commit_hash_base,
        commit_hash_head,
        check_merge_commits=options.check_merge_commits,
    )
    if commits is None:
        return 1

    # Process them
    infractions = process_commits(commits, options.check_merge_commits)

    # Check if there are any infractions
    result = check_infractions(infractions)

    if len(commits) == 0:
        logger.print('Warning: no commits were actually checked')

    return result


if __name__ == '__main__':  # pragma: no cover
    sys.exit(main())
