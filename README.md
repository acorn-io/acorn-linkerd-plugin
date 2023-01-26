# acorn-linkerd-plugin

Acorn linkerd plugin provides a way for acorn to integrate with linkerd service mesh. It mainly provides the current functionality to acorn.

1. Automatically add service mesh annotations to acorn workspaces. This ensures every acorn app namespace is annotated with the right annotation to be able to inject linkerd sidecar.

2. Kill linkerd sidecar container for Jobs when all other containers have completed. This is to address https://github.com/linkerd/linkerd2/issues/8006.

3. Automatically configure linkerd policies to ensure project level networking isolation between acorn projects.

### Build

```bash
make build
```

### Development

The best way to run the plugin is through acorn. Run 

```bash
acorn run --name controller -i .
```

### Production

```bash
acorn run ghcr.io/acorn-io/acorn-linkerd-plugin:main
```

## License
Copyright (c) 2022 [Acorn Labs, Inc.](http://acorn.io)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.