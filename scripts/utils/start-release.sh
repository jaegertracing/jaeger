#!/usr/bin/env bash
#Requires bash version to be >=4. Will add alternative for lower versions
set -euo pipefail
VERSION_TYPE=""

usage() {
  echo "Usage: $0 --type [v1|v2]"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -t|--type)
      VERSION_TYPE="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1"
      usage
      ;;
  esac
done

if [[ -z "${VERSION_TYPE}" ]]; then
  echo "Error: --type not provided."
  usage
fi

if [[ "${VERSION_TYPE}" != "v1" && "${VERSION_TYPE}" != "v2" ]]; then
  echo "Error: invalid --type value '${VERSION_TYPE}'. Must be 'v1' or 'v2'."
  usage
fi

if ! current_version=$(make "echo-${VERSION_TYPE}"); then
  echo "Error: Failed to fetch current version from make echo-${VERSION_TYPE}."
  exit 1
fi

current_version=$(echo "$current_version" | xargs)

clean_version="${current_version#v}"

IFS='.' read -r major minor patch <<< "$clean_version"

# Default to incrementing the patch number
patch=$((patch + 1))
suggested_version="${major}.${minor}.${patch}"
echo "Current ${VERSION_TYPE} version: ${current_version}"
read -e -p "New version: v" -i "${suggested_version}" user_version

new_version="v${user_version}"
echo "Using new version: ${new_version}"

exit 1;
