#!/usr/bin/env bash
#
# Copyright 2020 The Multicluster-Scheduler Authors.
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
#

set -euo pipefail

source test/e2e/aliases.sh

exec_test() {
  i=$1
  j=$2

  k $j label node --all a=b --overwrite
  k $i apply -f test/e2e/exec/test.yaml
  while [ $(k $i get pod -l job-name=exec | wc -l) = 0 ]; do sleep 1; done
  k $i wait pod -l job-name=exec --for=condition=ContainersReady
  k $i exec job/exec ls | grep bin
  k $i delete -f test/e2e/exec/test.yaml
  k $j label node --all a-
}

if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]]; then
  exec_test "${@}"
fi
