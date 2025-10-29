#!/usr/bin/env bash

# Copyright 2021 The Tekton Authors
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

set -o errexit
set -o nounset
set -o pipefail

source $(git rev-parse --show-toplevel)/vendor/github.com/tektoncd/plumbing/scripts/library.sh

boilerplate="$(git rev-parse --show-toplevel)/hack/boilerplate/boilerplate.go.txt"

# Pin code-generator version to match go.mod
go install k8s.io/code-generator/cmd/deepcopy-gen@v0.33.1

# Use GOBIN if set, otherwise fall back to GOPATH/bin
CODEGEN_BIN="${GOBIN:-${GOPATH}/bin}/deepcopy-gen"

${CODEGEN_BIN} \
  --output-file zz_generated.deepcopy.go \
  --go-header-file "${boilerplate}" \
  -i github.com/tektoncd/pruner/pkg/config

# Make sure our dependencies are up-to-date
${REPO_ROOT_DIR}/hack/update-deps.sh