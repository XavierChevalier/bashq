# Example: deploy and rollback

Two scripts that deploy a release of an app to a list of hosts and roll one
host back. They are written the way real scripts grow: a retry loop nested
inside the main loop, and a function copied from one script to the other.

```console
$ bashq check examples/deploy
examples/deploy/deploy.sh:12 warning deploy::main has a cognitive complexity of 17 (max 15) [cognitive-complexity]
examples/deploy/rollback.sh:8 warning 23 lines duplicate examples/deploy/deploy.sh:46-68 [duplication]

✗ 2 issues: cognitive-complexity 1, duplication 1 (2 files, 6 functions)
```

[Getting started](../../docs/getting-started.md) walks through both issues
and their fixes. The scripts are left as they are on purpose: the CLI tests
(`cmd/bashq/main_test.go`) check bashq's output on them.
