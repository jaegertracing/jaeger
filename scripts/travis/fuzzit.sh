#!/bin/bash

# Copyright (c) 2019 The Jaeger Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -xe

# Validate arguments
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <fuzz-type>"
    exit 1
fi

# Configure
NAME=jaeger
ROOT=.
TYPE=$1

# Setup
export GO111MODULE="off"
go get -u github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build
dep ensure -v
if [ ! -f fuzzit ]; then
    wget -q -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/latest/download/fuzzit_Linux_x86_64
    chmod a+x fuzzit
fi

# Fuzz
function fuzz {
    FUNC=Fuzz$1
    TARGET=$2
    DIR=$ROOT/$3
    go-fuzz-build -libfuzzer -func $FUNC -o fuzzer.a $DIR
    clang -fsanitize=fuzzer fuzzer.a -o fuzzer
    ./fuzzit create job --type $TYPE $NAME/$TARGET fuzzer
}
fuzz "" deserialize-zipkin model/converter/thrift/zipkin
fuzz "" deserialize-json cmd/collector/app/zipkin
fuzz "" agent cmd/agent/app
