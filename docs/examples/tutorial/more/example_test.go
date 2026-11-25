// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package more_test

import (
	"github.com/go-openapi/go-yaml/docs/examples/tutorial/internal/answer"
)

// Example_kyaml reads the Pod of the KYAML reference page: flow collections, double-quoted string values and trailing
// commas, all of it YAML.
func Example_kyaml() {
	answer.Print("testdata/kyaml.yaml")

	// Output:
	// {
	//   "apiVersion": "v1",
	//   "kind": "Pod",
	//   "metadata": {
	//     "name": "my-pod",
	//     "labels": {
	//       "app": "demo"
	//     }
	//   },
	//   "spec": {
	//     "containers": [
	//       {
	//         "name": "nginx",
	//         "image": "nginx:1.20"
	//       }
	//     ]
	//   }
	// }
}
