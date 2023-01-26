# acorn-linkerd-plugin

Acorn linkerd plugin provides a way for acorn to integrate with linkerd service mesh. It mainly provides the current functionality to acorn.

1. Automatically add service mesh annotations to acorn workspaces. This is done by annotations/labels propogagtion feature that propogates annotations and labels from projects to app namespaces.
2. Address an edge case where k8s pod can’t successfully complete once service mesh is enabled.
3. Automatically configure project level networking isolation between acorn projects.